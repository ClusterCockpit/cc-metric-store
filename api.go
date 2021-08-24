package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/lineprotocol"
	"github.com/gorilla/mux"
)

type HostData struct {
	Host  string               `json:"host"`
	Start int64                `json:"start"`
	Data  []lineprotocol.Float `json:"data"`
}

type MetricData struct {
	Hosts []HostData `json:"hosts"`
}

type TimeseriesResponse map[string]MetricData

type HostStats struct {
	Host    string             `json:"host"`
	Sampels int                `json:"sampels"`
	Avg     lineprotocol.Float `json:"avg"`
	Min     lineprotocol.Float `json:"min"`
	Max     lineprotocol.Float `json:"max"`
}

type MetricStats struct {
	Hosts []HostStats `json:"hosts"`
}

type StatsResponse map[string]MetricStats

func handleTimeseries(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster := vars["cluster"]
	from, err := strconv.ParseInt(vars["from"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := strconv.ParseInt(vars["to"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	values := r.URL.Query()
	hosts := values["host"]
	metrics := values["metric"]
	if len(hosts) < 1 || len(metrics) < 1 {
		http.Error(rw, "no hosts or metrics specified", http.StatusBadRequest)
		return
	}

	response := TimeseriesResponse{}
	store, ok := metricStores[vars["class"]]
	if !ok {
		http.Error(rw, "invalid class", http.StatusInternalServerError)
		return
	}

	for _, metric := range metrics {
		hostsdata := []HostData{}
		for _, host := range hosts {
			key := cluster + ":" + host
			data, start, err := store.GetMetric(key, metric, from, to)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			hostsdata = append(hostsdata, HostData{
				Host:  host,
				Start: start,
				Data:  data,
			})
		}
		response[metric] = MetricData{
			Hosts: hostsdata,
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(response)
	if err != nil {
		log.Println(err.Error())
	}
}

func handleStats(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster := vars["cluster"]
	from, err := strconv.ParseInt(vars["from"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := strconv.ParseInt(vars["to"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	values := r.URL.Query()
	hosts := values["host"]
	metrics := values["metric"]
	if len(hosts) < 1 || len(metrics) < 1 {
		http.Error(rw, "no hosts or metrics specified", http.StatusBadRequest)
		return
	}

	response := StatsResponse{}
	store, ok := metricStores[vars["class"]]
	if !ok {
		http.Error(rw, "invalid class", http.StatusInternalServerError)
		return
	}

	for _, metric := range metrics {
		hoststats := []HostStats{}
		for _, host := range hosts {
			key := cluster + ":" + host
			min, max := math.MaxFloat64, -math.MaxFloat64
			samples := 0

			sum, err := store.Reduce(key, metric, from, to, func(t int64, sum, x lineprotocol.Float) lineprotocol.Float {
				if math.IsNaN(float64(x)) {
					return sum
				}

				samples += 1
				min = math.Min(min, float64(x))
				max = math.Max(max, float64(x))
				return sum + x
			}, 0.)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			hoststats = append(hoststats, HostStats{
				Host:    host,
				Sampels: samples,
				Avg:     sum / lineprotocol.Float(samples),
				Min:     lineprotocol.Float(min),
				Max:     lineprotocol.Float(max),
			})
		}
		response[metric] = MetricStats{
			Hosts: hoststats,
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(response)
	if err != nil {
		log.Println(err.Error())
	}
}

func handlePeak(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster := vars["cluster"]
	store, ok := metricStores[vars["class"]]
	if !ok {
		http.Error(rw, "invalid class", http.StatusInternalServerError)
		return
	}

	response := store.Peak(cluster + ":")
	rw.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(rw).Encode(response)
	if err != nil {
		log.Println(err.Error())
	}
}

func StartApiServer(address string, done chan bool) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/{cluster}/{class:(?:node|socket|cpu)}/{from:[0-9]+}/{to:[0-9]+}/timeseries", handleTimeseries)
	r.HandleFunc("/api/{cluster}/{class:(?:node|socket|cpu)}/{from:[0-9]+}/{to:[0-9]+}/stats", handleStats)
	r.HandleFunc("/api/{cluster}/{class:(?:node|socket|cpu)}/peak", handlePeak)

	server := &http.Server{
		Handler:      r,
		Addr:         address,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	go func() {
		log.Printf("API http endpoint listening on '%s'\n", address)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Println(err)
		}
	}()

	for {
		_ = <-done
		err := server.Shutdown(context.Background())
		log.Println("API server shut down")
		return err
	}
}
