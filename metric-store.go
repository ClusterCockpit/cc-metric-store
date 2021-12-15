package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
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
	HttpApiAddress    string                  `json:"http-api-address"`
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

func handleLine(dec *lineprotocol.Decoder) error {
	for dec.Next() {
		measurement, err := dec.Measurement()
		if err != nil {
			return err
		}

		var cluster, host, typeName, typeId string
		for {
			key, val, err := dec.NextTag()
			if err != nil {
				return err
			}
			if key == nil {
				break
			}

			switch string(key) {
			case "cluster":
				cluster = string(val)
			case "hostname":
				host = string(val)
			case "type":
				typeName = string(val)
			case "type-id":
				typeId = string(val)
			case "unit", "group":
				// Ignore... (Important only for ganglia)
			default:
				return fmt.Errorf("unkown tag: '%s' (value: '%s')", string(key), string(val))
			}
		}

		selector := make([]string, 0, 3)
		selector = append(selector, cluster)
		selector = append(selector, host)
		if len(typeId) > 0 {
			selector = append(selector, typeName+typeId)
		}

		var value Float
		for {
			key, val, err := dec.NextField()
			if err != nil {
				return err
			}

			if key == nil {
				break
			}

			if string(key) != "value" {
				return fmt.Errorf("unkown field: '%s' (value: %#v)", string(key), val)
			}

			if val.Kind() == lineprotocol.Float {
				value = Float(val.FloatV())
			} else if val.Kind() == lineprotocol.Int {
				value = Float(val.IntV())
			} else {
				return fmt.Errorf("unsupported value type in message: %s", val.Kind().String())
			}
		}

		t, err := dec.Time(lineprotocol.Second, time.Now())
		if err != nil {
			return err
		}

		// log.Printf("write: %s (%v) -> %v\n", string(measurement), selector, value)
		if err := memoryStore.Write(selector, t.Unix(), []Metric{
			{Name: string(measurement), Value: value},
		}); err != nil {
			return err
		}
	}
	return nil
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

	restoreFrom := startupTime.Add(-time.Duration(conf.Checkpoints.Restore) * time.Second)
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
		err := StartApiServer(conf.HttpApiAddress, ctx)
		if err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()

	if len(conf.Nats) != 0 {
		wg.Add(1)

		go func() {
			// err := ReceiveNats(conf.Nats, handleLine, runtime.NumCPU()-1, ctx)
			err := ReceiveNats(conf.Nats, handleLine, 1, ctx)

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
