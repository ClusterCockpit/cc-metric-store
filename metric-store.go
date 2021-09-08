package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
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
	Metrics                 map[string]MetricConfig `json:"metrics"`
	RetentionHours          int                     `json:"retention-hours"`
	RestoreLastHours        int                     `json:"restore-last-hours"`
	CheckpointIntervalHours int                     `json:"checkpoint-interval-hours"`
	ArchiveRoot             string                  `json:"archive-root"`
	Nats                    string                  `json:"nats"`
}

var conf Config
var memoryStore *MemoryStore = nil
var lastCheckpoint time.Time

func loadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
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

func main() {
	startupTime := time.Now()
	conf = loadConfiguration("config.json")

	memoryStore = NewMemoryStore(conf.Metrics)

	if conf.ArchiveRoot != "" && conf.RestoreLastHours > 0 {
		d := time.Duration(conf.RestoreLastHours) * time.Hour
		from := startupTime.Add(-d).Unix()
		log.Printf("Restoring data since %d from '%s'...\n", from, conf.ArchiveRoot)
		files, err := memoryStore.FromArchive(conf.ArchiveRoot, from)
		if err != nil {
			log.Printf("Loading archive failed: %s\n", err.Error())
		} else {
			log.Printf("Archive loaded (%d files)\n", files)
		}
	}

	ctx, shutdown := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		_ = <-sigs
		log.Println("Shuting down...")
		shutdown()
	}()

	lastCheckpoint = startupTime
	if conf.ArchiveRoot != "" && conf.CheckpointIntervalHours > 0 {
		wg.Add(3)
		go func() {
			d := time.Duration(conf.CheckpointIntervalHours) * time.Hour
			ticks := time.Tick(d)
			for {
				select {
				case <-ctx.Done():
					wg.Done()
					return
				case <-ticks:
					log.Println("Start making checkpoint...")
					now := time.Now()
					n, err := memoryStore.ToArchive(conf.ArchiveRoot, lastCheckpoint.Unix(), now.Unix())
					if err != nil {
						log.Printf("Making checkpoint failed: %s\n", err.Error())
					} else {
						log.Printf("Checkpoint successfull (%d files written)\n", n)
					}
					lastCheckpoint = now

					if conf.RetentionHours > 0 {
						log.Println("Freeing up memory...")
						t := now.Add(-time.Duration(conf.RetentionHours) * time.Hour)
						freed, err := memoryStore.Free(Selector{}, t.Unix())
						if err != nil {
							log.Printf("Freeing up memory failed: %s\n", err.Error())
						}
						log.Printf("%d values freed\n", freed)
					}
				}
			}
		}()
	} else {
		wg.Add(2)
	}

	go func() {
		err := StartApiServer(":8080", ctx)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	go func() {
		err := ReceiveNats(conf.Nats, handleLine, runtime.NumCPU()-1, ctx)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	wg.Wait()

	if conf.ArchiveRoot != "" {
		log.Printf("Writing to '%s'...\n", conf.ArchiveRoot)
		files, err := memoryStore.ToArchive(conf.ArchiveRoot, lastCheckpoint.Unix(), time.Now().Unix())
		if err != nil {
			log.Printf("Writing to archive failed: %s\n", err.Error())
		}
		log.Printf("Done! (%d files written)\n", files)
	}
}
