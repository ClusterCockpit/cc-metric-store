package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

const token = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"
const ccmsurl = "http://localhost:8081/api/write"
const cluster = "fakedev"
const sockets = 2
const cpus = 8
const freq = 15 * time.Second

var hosts = []string{"fake001", "fake002", "fake003", "fake004", "fake005"}
var metrics = []struct {
	Name     string
	Type     string
	AvgValue float64
}{
	{"flops_any", "cpu", 10.0},
	{"mem_bw", "socket", 50.0},
	{"ipc", "cpu", 1.25},
	{"cpu_load", "node", 4},
	{"mem_used", "node", 20},
}

var states = make([]float64, 0)

func send(client *http.Client, t int64) {
	msg := &bytes.Buffer{}

	i := 0
	for _, host := range hosts {
		for _, metric := range metrics {
			n := 1
			if metric.Type == "socket" {
				n = sockets
			} else if metric.Type == "cpu" {
				n = cpus
			}

			for j := 0; j < n; j++ {
				fmt.Fprintf(msg, "%s,cluster=%s,host=%s,type=%s", metric.Name, cluster, host, metric.Type)
				if metric.Type == "socket" {
					fmt.Fprintf(msg, ",type-id=%d", j)
				} else if metric.Type == "cpu" {
					fmt.Fprintf(msg, ",type-id=%d", j)
				}

				x := metric.AvgValue + math.Sin(states[i])*(metric.AvgValue/10.)
				states[i] += 0.1
				fmt.Fprintf(msg, " value=%f ", x)

				fmt.Fprintf(msg, "%d\n", t)
				i++
			}
		}
	}

	req, _ := http.NewRequest(http.MethodPost, ccmsurl, msg)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := client.Do(req)
	if err != nil {
		log.Print(err)
		return
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		log.Printf("%s: %s", res.Status, string(body))
	}
}

func main() {
	for range hosts {
		for _, m := range metrics {
			n := 1
			if m.Type == "socket" {
				n = sockets
			} else if m.Type == "cpu" {
				n = cpus
			}

			for i := 0; i < n; i++ {
				states = append(states, rand.Float64()*100)
			}
		}
	}

	client := &http.Client{}

	i := 0
	for t := range time.Tick(freq) {
		log.Printf("tick... (#%d)", i)
		i++

		send(client, t.Unix())
	}
}
