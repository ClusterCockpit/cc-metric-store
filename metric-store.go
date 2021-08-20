package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ClusterCockpit/cc-metric-store/lineprotocol"
)

type MetricStore interface {
	AddMetrics(key string, ts int64, metrics []lineprotocol.Metric) error
	GetMetric(key string, metric string, from int64, to int64) ([]float64, int64, error)
}

type Config struct {
	MetricClasses map[string]struct {
		Frequency int      `json:"frequency"`
		Metrics   []string `json:"metrics"`
	} `json:"metrics"`
}

var conf Config
var metricStores map[string]MetricStore = map[string]MetricStore{}

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

// TODO: Change MetricStore API so that we do not have to do string concat?
// Nested hashmaps could be an alternative.
func buildKey(line *lineprotocol.Line) (string, error) {
	cluster, ok := line.Tags["cluster"]
	if !ok {
		return "", errors.New("missing cluster tag")
	}

	host, ok := line.Tags["host"]
	if !ok {
		return "", errors.New("missing host tag")
	}

	cpu, ok := line.Tags["cpu"]
	if ok {
		return cluster + ":" + host + ":" + cpu, nil
	}

	return cluster + ":" + host, nil
}

func handleLine(line *lineprotocol.Line) {
	log.Printf("line: %v (t=%d)\n", line, line.Ts.Unix())

	store := metricStores[line.Measurement]
	key, err := buildKey(line)
	if err != nil {
		log.Println(err)
	}

	err = store.AddMetrics(key, line.Ts.Unix(), line.Fields)
}

func main() {
	conf = loadConfiguration("config.json")

	for class, info := range conf.MetricClasses {
		metricStores[class] = newMemoryStore(info.Metrics, 1000, info.Frequency)
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		_ = <-sigs
		done <- true
		close(done)
		log.Println("shuting down")
	}()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		StartApiServer(":8080", done)
		wg.Done()
	}()

	err := lineprotocol.ReceiveNats("nats://localhost:4222", handleLine, done)
	if err != nil {
		log.Fatal(err)
	}

	wg.Wait()
}
