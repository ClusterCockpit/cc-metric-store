package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/internal/api"
	"github.com/ClusterCockpit/cc-metric-store/internal/api/apiv1"
	"github.com/ClusterCockpit/cc-metric-store/internal/memstore"
	"github.com/ClusterCockpit/cc-metric-store/internal/types"
	"github.com/golang/snappy"
	"github.com/google/gops/agent"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

// For aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func (as *AggregationStrategy) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "":
		*as = NoAggregation
	case "sum":
		*as = SumAggregation
	case "avg":
		*as = AvgAggregation
	default:
		return fmt.Errorf("invalid aggregation strategy: %#v", str)
	}
	return nil
}

type HttpConfig struct {
	// Address to bind to, for example "0.0.0.0:8081"
	Address string `json:"address"`

	// If not the empty string, use https with this as the certificate file
	CertFile string `json:"https-cert-file"`

	// If not the empty string, use https with this as the key file
	KeyFile string `json:"https-key-file"`
}

type Config struct {
	Metrics           map[string]types.MetricConfig `json:"metrics"`
	RetentionInMemory string                        `json:"retention-in-memory"`
	Nats              []*api.NatsConfig             `json:"nats"`
	JwtPublicKey      string                        `json:"jwt-public-key"`
	HttpConfig        *HttpConfig                   `json:"http-api"`
	Checkpoints       struct {
		Interval         string `json:"interval"`
		RootDir          string `json:"directory"`
		Restore          string `json:"restore"`
		Binary           bool   `json:"binary-enabled"`
		BinaryCompressed bool   `json:"binary-compressed"`
	} `json:"checkpoints"`
	Archive struct {
		Interval      string `json:"interval"`
		RootDir       string `json:"directory"`
		DeleteInstead bool   `json:"delete-instead"`
	} `json:"archive"`
	Debug struct {
		EnableGops bool `json:"gops"`
	} `json:"debug"`
}

var conf Config
var memoryStore *memstore.MemoryStore = nil
var lastCheckpoint time.Time

func loadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer configFile.Close()
	dec := json.NewDecoder(configFile)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&config); err != nil {
		log.Fatal(err)
	}
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
				freed := memoryStore.Free(t.Unix())
				log.Printf("done: %d buffers freed\n", freed)
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
				if conf.Checkpoints.Binary {
					if err := makeBinaryCheckpoint(lastCheckpoint.Unix(), now.Unix()); err != nil {
						log.Printf("checkpointing failed: %s\n", err.Error())
					} else {
						log.Printf("done!")
					}
				} else {
					n, err := memoryStore.ToJSONCheckpoint(conf.Checkpoints.RootDir,
						lastCheckpoint.Unix(), now.Unix())
					if err != nil {
						log.Printf("checkpointing failed: %s\n", err.Error())
					} else {
						log.Printf("done: %d checkpoint files created\n", n)
						lastCheckpoint = now
					}
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
	conf = loadConfiguration(configFile)
	memoryStore = memstore.NewMemoryStore(conf.Metrics)

	if enableGopsAgent || conf.Debug.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			log.Fatal(err)
		}
	}

	d, err := time.ParseDuration(conf.Checkpoints.Restore)
	if err != nil {
		log.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	log.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	if conf.Checkpoints.Binary {
		if err := loadBinaryCheckpoints(restoreFrom.Unix()); err != nil {
			log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
		} else {
			loadedData := memoryStore.SizeInBytes() / 1024 / 1024 // In MB
			log.Printf("Checkpoints loaded (%d MB, that took %fs)\n", loadedData, time.Since(startupTime).Seconds())
		}
	} else {
		files, err := memoryStore.FromJSONCheckpoint(conf.Checkpoints.RootDir, restoreFrom.Unix())
		if err != nil {
			log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
		} else {
			loadedData := memoryStore.SizeInBytes() / 1024 / 1024 // In MB
			log.Printf("Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
		}
	}

	// Try to use less memory by forcing a GC run here and then
	// lowering the target percentage. The default of 100 means
	// that only once the ratio of new allocations execeds the
	// previously active heap, a GC is triggered.
	// Forcing a GC here will set the "previously active heap"
	// to a minumum.
	runtime.GC()
	if os.Getenv("GOGC") == "" {
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

			log.Println("Shuting down...")
			shutdown()
		}
	}()

	intervals(&wg, ctx)

	wg.Add(1)

	httpApi := apiv1.HttpApi{
		MemoryStore: memoryStore,
		PublicKey:   conf.JwtPublicKey,
		Address:     conf.HttpConfig.Address,
		CertFile:    conf.HttpConfig.CertFile,
		KeyFile:     conf.HttpConfig.KeyFile,
	}

	go func() {
		err := httpApi.StartServer(ctx)
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
				err := api.ReceiveNats(nc, func(d *lineprotocol.Decoder, s string) error {
					return api.DecodeLine(memoryStore, d, s)
				}, 1, ctx)

				if err != nil {
					log.Fatal(err)
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()

	log.Printf("Writing checkpoint...\n")
	if conf.Checkpoints.Binary {
		if err := makeBinaryCheckpoint(lastCheckpoint.Unix(), time.Now().Unix()+60); err != nil {
			log.Printf("Writing checkpoint failed: %s\n", err.Error())
		}
		log.Printf("Done!")
	} else {
		files, err := memoryStore.ToJSONCheckpoint(conf.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
		if err != nil {
			log.Printf("Writing checkpoint failed: %s\n", err.Error())
		}
		log.Printf("Done! (%d files written)\n", files)
	}
}

func loadBinaryCheckpoints(from int64) error {
	dir, err := os.ReadDir(conf.Checkpoints.RootDir)
	if err != nil {
		return err
	}

	for _, entry := range dir {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".data") {
			continue
		}

		t, err := strconv.ParseInt(strings.TrimSuffix(entry.Name(), ".data"), 10, 64)
		if err != nil {
			return err
		}

		if t < from {
			continue
		}

		log.Printf("loading checkpoint file %s/%s...", conf.Checkpoints.RootDir, entry.Name())
		f, err := os.Open(filepath.Join(conf.Checkpoints.RootDir, entry.Name()))
		if err != nil {
			return err
		}

		var reader io.Reader = bufio.NewReader(f)
		if conf.Checkpoints.BinaryCompressed {
			reader = snappy.NewReader(reader)
		}

		if err := memoryStore.LoadCheckpoint(reader); err != nil {
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}
	}

	return nil
}

func makeBinaryCheckpoint(from, to int64) error {
	filename := filepath.Join(conf.Checkpoints.RootDir, fmt.Sprintf("%d.data", from))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if conf.Checkpoints.BinaryCompressed {
		compressor := snappy.NewBufferedWriter(f)
		if err := memoryStore.SaveCheckpoint(from, to, compressor); err != nil {
			return err
		}
		if err := compressor.Flush(); err != nil {
			return err
		}
	} else {
		if err := memoryStore.SaveCheckpoint(from, to, f); err != nil {
			return err
		}
	}

	return nil
}
