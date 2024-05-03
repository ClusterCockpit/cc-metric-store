package main

import (
	"bufio"
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/internal/api"
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/ClusterCockpit/cc-metric-store/internal/memstore"
	"github.com/google/gops/agent"
)

var (
	conf           config.Config
	memoryStore    *memstore.MemoryStore = nil
	lastCheckpoint time.Time
)

var (
	debugDumpLock sync.Mutex
	debugDump     io.Writer = io.Discard
)

func intervals(wg *sync.WaitGroup, ctx context.Context) {
	wg.Add(3)
	// go func() {
	// 	defer wg.Done()
	// 	ticks := time.Tick(30 * time.Minute)
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case <-ticks:
	// 			runtime.GC()
	// 		}
	// 	}
	// }()

	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(conf.RetentionInMemory)
		if err != nil {
			log.Fatal(err)
		}
		if d <= 0 {
			return
		}

		ticks := time.Tick(d / 2)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				t := time.Now().Add(-d)
				log.Printf("start freeing buffers (older than %s)...\n", t.Format(time.RFC3339))
				freed, err := memoryStore.Free(nil, t.Unix())
				if err != nil {
					log.Printf("freeing up buffers failed: %s\n", err.Error())
				} else {
					log.Printf("done: %d buffers freed\n", freed)
				}
			}
		}
	}()

	lastCheckpoint = time.Now()
	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(conf.Checkpoints.Interval)
		if err != nil {
			log.Fatal(err)
		}
		if d <= 0 {
			return
		}

		ticks := time.Tick(d)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				log.Printf("start checkpointing (starting at %s)...\n", lastCheckpoint.Format(time.RFC3339))
				now := time.Now()
				n, err := memoryStore.ToCheckpoint(conf.Checkpoints.RootDir,
					lastCheckpoint.Unix(), now.Unix())
				if err != nil {
					log.Printf("checkpointing failed: %s\n", err.Error())
				} else {
					log.Printf("done: %d checkpoint files created\n", n)
					lastCheckpoint = now
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(conf.Archive.Interval)
		if err != nil {
			log.Fatal(err)
		}
		if d <= 0 {
			return
		}

		ticks := time.Tick(d)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				t := time.Now().Add(-d)
				log.Printf("start archiving checkpoints (older than %s)...\n", t.Format(time.RFC3339))
				n, err := memstore.ArchiveCheckpoints(conf.Checkpoints.RootDir, conf.Archive.RootDir, t.Unix(), conf.Archive.DeleteInstead)
				if err != nil {
					log.Printf("archiving failed: %s\n", err.Error())
				} else {
					log.Printf("done: %d files zipped and moved to archive\n", n)
				}
			}
		}
	}()
}

func main() {
	var configFile string
	var enableGopsAgent bool
	flag.StringVar(&configFile, "config", "./config.json", "configuration file")
	flag.BoolVar(&enableGopsAgent, "gops", false, "Listen via github.com/google/gops/agent")
	flag.Parse()

	startupTime := time.Now()
	conf = config.LoadConfiguration(configFile)
	memoryStore = memstore.NewMemoryStore(conf.Metrics)

	if enableGopsAgent || conf.Debug.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			log.Fatal(err)
		}
	}

	if conf.Debug.DumpToFile != "" {
		f, err := os.Create(conf.Debug.DumpToFile)
		if err != nil {
			log.Fatal(err)
		}

		debugDump = f
	}

	d, err := time.ParseDuration(conf.Checkpoints.Restore)
	if err != nil {
		log.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	log.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := memoryStore.FromCheckpoint(conf.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := memoryStore.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
	} else {
		log.Printf("Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
	}

	// Try to use less memory by forcing a GC run here and then
	// lowering the target percentage. The default of 100 means
	// that only once the ratio of new allocations execeds the
	// previously active heap, a GC is triggered.
	// Forcing a GC here will set the "previously active heap"
	// to a minumum.
	runtime.GC()
	if loadedData > 1000 && os.Getenv("GOGC") == "" {
		debug.SetGCPercent(10)
	}

	ctx, shutdown := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGUSR1 {
				memoryStore.DebugDump(bufio.NewWriter(os.Stdout), nil)
				continue
			}

			log.Println("Shutting down...")
			shutdown()
		}
	}()

	intervals(&wg, ctx)

	wg.Add(1)

	go func() {
		err := api.StartApiServer(ctx, conf.HttpConfig)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	if conf.Nats != nil {
		for _, natsConf := range conf.Nats {
			// TODO: When multiple nats configs share a URL, do a single connect.
			wg.Add(1)
			nc := natsConf
			go func() {
				// err := ReceiveNats(conf.Nats, decodeLine, runtime.NumCPU()-1, ctx)
				err := api.ReceiveNats(nc, decodeLine, 1, ctx)
				if err != nil {
					log.Fatal(err)
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()

	log.Printf("Writing to '%s'...\n", conf.Checkpoints.RootDir)
	files, err = memoryStore.ToCheckpoint(conf.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	if err != nil {
		log.Printf("Writing checkpoint failed: %s\n", err.Error())
	}
	log.Printf("Done! (%d files written)\n", files)

	if closer, ok := debugDump.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Printf("error: %s", err.Error())
		}
	}
}
