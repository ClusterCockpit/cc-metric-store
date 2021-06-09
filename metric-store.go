package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	nats "github.com/nats-io/nats.go"
)

type Config struct {
	MemoryStore struct {
		Duration string `json:"duration"`
	} `json:"memory_store"`
	FileStore struct {
		Duration string `json:"duration"`
	} `json:"file_store"`
	Root      string   `json:"root"`
	Frequency int      `json:"frequency"`
	Metrics   []string `json:"metrics"`
}

type MetricData struct {
	Name   string
	Values []float64
}

type Metric struct {
	Name  string
	Value float64
}

type message struct {
	Ts     int64
	Tags   []string
	Fields []Metric
}

var Conf Config

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

func main() {

	Conf = loadConfiguration("config.json")

	// Connect to a server
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	var msgBuffer bytes.Buffer
	dec := gob.NewDecoder(&msgBuffer)

	// Use a WaitGroup to wait for a message to arrive
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Subscribe
	if _, err := nc.Subscribe("updates", func(m *nats.Msg) {
		log.Println(m.Subject)
		var p message
		err = dec.Decode(&p)
		if err != nil {
			log.Fatal("decode error 1:", err)
		}
	}); err != nil {
		log.Fatal(err)
	}

	// Wait for a message to come in
	wg.Wait()
}
