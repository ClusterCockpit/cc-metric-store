package main

import (
	"context"
	"encoding/json"
	"log"
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

type TimeseriesNodeResponse map[string]MetricData

func handleTimeseriesNode(rw http.ResponseWriter, r *http.Request) {
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

	response := TimeseriesNodeResponse{}
	store := metricStores["node"]
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

func StartApiServer(address string, done chan bool) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/{cluster}/timeseries/node/{from:[0-9]+}/{to:[0-9]+}", handleTimeseriesNode)

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
