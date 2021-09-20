package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type MetricConfig struct {
	Frequency   int64  `json:"frequency"`
	Aggregation string `json:"aggregation"`
	Scope       string `json:"scope"`
}

type Config struct {
	Metrics           map[string]MetricConfig `json:"metrics"`
	RetentionInMemory int                     `json:"retention-in-memory"`
	Nats              string                  `json:"nats"`
	JwtPublicKey      string                  `json:"jwt-public-key"`
	Checkpoints       struct {
		Interval int    `json:"interval"`
		RootDir  string `json:"directory"`
		Restore  int    `json:"restore"`
	} `json:"checkpoints"`
	Archive struct {
		Interval int    `json:"interval"`
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

func handleLine(line *Line) {
	cluster, ok := line.Tags["cluster"]
	if !ok {
		log.Println("'cluster' tag missing")
		return
	}

	host, ok := line.Tags["host"]
	if !ok {
		log.Println("'host' tag missing")
		return
	}

	selector := []string{cluster, host}
	if id, ok := line.Tags[line.Measurement]; ok {
		selector = append(selector, line.Measurement+id)
	}

	ts := line.Ts.Unix()
	// log.Printf("ts=%d, tags=%v\n", ts, selector)
	err := memoryStore.Write(selector, ts, line.Fields)
	if err != nil {
		log.Printf("error: %s\n", err.Error())
	}
}

func intervals(wg *sync.WaitGroup, ctx context.Context) {
	wg.Add(3)
	go func() {
		defer wg.Done()
		d := time.Duration(conf.RetentionInMemory) * time.Second
		if d <= 0 {
			return
		}
		ticks := time.Tick(d / 2)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				log.Println("Freeing up memory...")
				t := time.Now().Add(-d)
				freed, err := memoryStore.Free(Selector{}, t.Unix())
				if err != nil {
					log.Printf("Freeing up memory failed: %s\n", err.Error())
				} else {
					log.Printf("%d buffers freed\n", freed)
				}
			}
		}
	}()

	lastCheckpoint = time.Now()
	go func() {
		defer wg.Done()
		d := time.Duration(conf.Checkpoints.Interval) * time.Second
		if d <= 0 {
			return
		}
		ticks := time.Tick(d)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				log.Println("Checkpoint creation started...")
				now := time.Now()
				n, err := memoryStore.ToCheckpoint(conf.Checkpoints.RootDir,
					lastCheckpoint.Unix(), now.Unix())
				if err != nil {
					log.Printf("Checkpoint creation failed: %s\n", err.Error())
				} else {
					log.Printf("Checkpoint finished (%d files)\n", n)
					lastCheckpoint = now
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		d := time.Duration(conf.Archive.Interval) * time.Second
		if d <= 0 {
			return
		}
		ticks := time.Tick(d)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				log.Println("Start zipping and deleting old checkpoints...")
				t := time.Now().Add(-d)
				err := ArchiveCheckpoints(conf.Checkpoints.RootDir, conf.Archive.RootDir, t.Unix())
				if err != nil {
					log.Printf("Archiving failed: %s\n", err.Error())
				} else {
					log.Println("Archiving checkpoints completed!")
				}
			}
		}
	}()
}

func main() {
	startupTime := time.Now()
	conf = loadConfiguration("config.json")
	memoryStore = NewMemoryStore(conf.Metrics)

	restoreFrom := startupTime.Add(-time.Duration(conf.Checkpoints.Restore))
	files, err := memoryStore.FromCheckpoint(conf.Checkpoints.RootDir, restoreFrom.Unix())
	if err != nil {
		log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
	} else {
		log.Printf("Checkpoints loaded (%d files)\n", files)
	}

	ctx, shutdown := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("Shuting down...")
		shutdown()
	}()

	intervals(&wg, ctx)

	wg.Add(2)

	go func() {
		err := StartApiServer(":8080", ctx)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	go func() {
		// err := ReceiveNats(conf.Nats, handleLine, runtime.NumCPU()-1, ctx)
		err := ReceiveNats(conf.Nats, handleLine, 1, ctx)

		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	wg.Wait()

	log.Printf("Writing to '%s'...\n", conf.Checkpoints.RootDir)
	files, err = memoryStore.ToCheckpoint(conf.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	if err != nil {
		log.Printf("Writing checkpoint failed: %s\n", err.Error())
	}
	log.Printf("Done! (%d files written)\n", files)
}
