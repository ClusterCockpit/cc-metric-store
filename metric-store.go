package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type MetricConfig struct {
	// Interval in seconds at which measurements will arive.
	Frequency int64 `json:"frequency"`

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation string `json:"aggregation"`

	// Unused for now.
	Scope string `json:"scope"`
}

type HttpConfig struct {
	// Address to bind to, for example "0.0.0.0:8081"
	Address string `json:"address"`

	// If not the empty string, use https with this as the certificate file
	CertFile string `json:"https-cert-file"`

	// If not the empty string, use https with this as the key file
	KeyFile string `json:"https-key-file"`
}

type NatsConfig struct {
	// Address of the nats server
	Address string `json:"address"`
}

type Config struct {
	Metrics           map[string]MetricConfig `json:"metrics"`
	RetentionInMemory string                  `json:"retention-in-memory"`
	Nats              *NatsConfig             `json:"nats"`
	JwtPublicKey      string                  `json:"jwt-public-key"`
	HttpConfig        *HttpConfig             `json:"http-api"`
	Checkpoints       struct {
		Interval string `json:"interval"`
		RootDir  string `json:"directory"`
		Restore  string `json:"restore"`
	} `json:"checkpoints"`
	Archive struct {
		Interval string `json:"interval"`
		RootDir  string `json:"directory"`
	} `json:"archive"`
}

var conf Config
var memoryStore *MemoryStore = nil
var lastCheckpoint time.Time

func loadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return config
}

func intervals(wg *sync.WaitGroup, ctx context.Context) {
	wg.Add(3)
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
				freed, err := memoryStore.Free(Selector{}, t.Unix())
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
				n, err := ArchiveCheckpoints(conf.Checkpoints.RootDir, conf.Archive.RootDir, t.Unix())
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
	flag.StringVar(&configFile, "config", "./config.json", "configuration file")
	flag.Parse()

	startupTime := time.Now()
	conf = loadConfiguration(configFile)
	memoryStore = NewMemoryStore(conf.Metrics)

	d, err := time.ParseDuration(conf.Checkpoints.Restore)
	if err != nil {
		log.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	log.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := memoryStore.FromCheckpoint(conf.Checkpoints.RootDir, restoreFrom.Unix())
	if err != nil {
		log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
	} else {
		log.Printf("Checkpoints loaded (%d files, that took %dms)\n", files, time.Since(startupTime).Milliseconds())
	}

	ctx, shutdown := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	go func() {
		for {
			sig := <-sigs
			if sig == syscall.SIGUSR1 {
				memoryStore.DebugDump(bufio.NewWriter(os.Stdout))
				continue
			}

			log.Println("Shuting down...")
			shutdown()
		}
	}()

	intervals(&wg, ctx)

	wg.Add(1)

	go func() {
		err := StartApiServer(ctx, conf.HttpConfig)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	if conf.Nats != nil {
		wg.Add(1)

		go func() {
			// err := ReceiveNats(conf.Nats, decodeLine, runtime.NumCPU()-1, ctx)
			err := ReceiveNats(conf.Nats, decodeLine, 1, ctx)

			if err != nil {
				log.Fatal(err)
			}
			wg.Done()
		}()
	}

	wg.Wait()

	log.Printf("Writing to '%s'...\n", conf.Checkpoints.RootDir)
	files, err = memoryStore.ToCheckpoint(conf.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	if err != nil {
		log.Printf("Writing checkpoint failed: %s\n", err.Error())
	}
	log.Printf("Done! (%d files written)\n", files)
}
