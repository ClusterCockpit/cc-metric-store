package main

import (
	"bufio"
	"context"
	"flag"
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
	"github.com/ClusterCockpit/cc-metric-store/internal/memorystore"
	"github.com/google/gops/agent"
)

func main() {
	var configFile string
	var enableGopsAgent bool
	flag.StringVar(&configFile, "config", "./config.json", "configuration file")
	flag.BoolVar(&enableGopsAgent, "gops", false, "Listen via github.com/google/gops/agent")
	flag.Parse()

	startupTime := time.Now()
	config.Init(configFile)
	memorystore.Init(config.Keys.Metrics)
	ms := memorystore.GetMemoryStore()

	if enableGopsAgent || config.Keys.Debug.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			log.Fatal(err)
		}
	}

	d, err := time.ParseDuration(config.Keys.Checkpoints.Restore)
	if err != nil {
		log.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	log.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := ms.FromCheckpoint(config.Keys.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := ms.SizeInBytes() / 1024 / 1024 // In MB
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
				ms.DebugDump(bufio.NewWriter(os.Stdout), nil)
				continue
			}

			log.Println("Shutting down...")
			shutdown()
		}
	}()

	wg.Add(3)

	memorystore.Retention(&wg, ctx)
	memorystore.Checkpointing(&wg, ctx)
	memorystore.Archiving(&wg, ctx)

	wg.Add(1)

	go func() {
		err := api.StartApiServer(ctx, config.Keys.HttpConfig)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	if config.Keys.Nats != nil {
		for _, natsConf := range config.Keys.Nats {
			// TODO: When multiple nats configs share a URL, do a single connect.
			wg.Add(1)
			nc := natsConf
			go func() {
				// err := ReceiveNats(conf.Nats, decodeLine, runtime.NumCPU()-1, ctx)
				err := api.ReceiveNats(nc, ms, 1, ctx)
				if err != nil {
					log.Fatal(err)
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()
	memorystore.Shutdown()
}
