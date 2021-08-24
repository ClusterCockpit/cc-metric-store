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
	GetMetric(key string, metric string, from int64, to int64) ([]lineprotocol.Float, int64, error)
	Reduce(key, metric string, from, to int64, f func(t int64, sum, x lineprotocol.Float) lineprotocol.Float, initialX lineprotocol.Float) (lineprotocol.Float, error)
	Peak(prefix string) map[string]map[string]lineprotocol.Float
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

	socket, ok := line.Tags["socket"]
	if ok {
		return cluster + ":" + host + ":s" + socket, nil
	}

	cpu, ok := line.Tags["cpu"]
	if ok {
		return cluster + ":" + host + ":c" + cpu, nil
	}

	return cluster + ":" + host, nil
}

func handleLine(line *lineprotocol.Line) {
	store, ok := metricStores[line.Measurement]
	if !ok {
		log.Printf("unkown class: '%s'\n", line.Measurement)
		return
	}

	key, err := buildKey(line)
	if err != nil {
		log.Println(err)
		return
	}

	// log.Printf("t=%d, key='%s', values=%v\n", line.Ts.Unix(), key, line.Fields)
	log.Printf("new data: t=%d, key='%s'", line.Ts.Unix(), key)
	err = store.AddMetrics(key, line.Ts.Unix(), line.Fields)
	if err != nil {
		log.Println(err)
	}
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
