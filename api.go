package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// Example:
//	{
//		"metrics": ["flops_sp", "flops_dp"]
//		"selectors": [["emmy", "host123", "cpu", "0"], ["emmy", "host123", "cpu", "1"]]
//	}
type ApiRequestBody struct {
	Metrics   []string   `json:"metrics"`
	Selectors [][]string `json:"selectors"`
}

type ApiMetricData struct {
	From int64   `json:"from"`
	To   int64   `json:"to"`
	Data []Float `json:"data"`
}

type ApiStatsData struct {
	From    int64 `json:"from"`
	To      int64 `json:"to"`
	Samples int   `json:"samples"`
	Avg     Float `json:"avg"`
	Min     Float `json:"min"`
	Max     Float `json:"max"`
}

func handleTimeseries(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
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

	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyDec := json.NewDecoder(r.Body)
	var reqBody ApiRequestBody
	err = bodyDec.Decode(&reqBody)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	res := make([]map[string]ApiMetricData, 0, len(reqBody.Selectors))
	for _, selector := range reqBody.Selectors {
		metrics := make(map[string]ApiMetricData)
		for _, metric := range reqBody.Metrics {
			data, f, t, err := memoryStore.Read(selector, metric, from, to)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			metrics[metric] = ApiMetricData{
				From: f,
				To:   t,
				Data: data,
			}
		}
		res = append(res, metrics)
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(res)
	if err != nil {
		log.Println(err.Error())
	}
}

func handleStats(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
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

	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyDec := json.NewDecoder(r.Body)
	var reqBody ApiRequestBody
	err = bodyDec.Decode(&reqBody)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	res := make([]map[string]ApiStatsData, 0, len(reqBody.Selectors))
	for _, selector := range reqBody.Selectors {
		metrics := make(map[string]ApiStatsData)
		for _, metric := range reqBody.Metrics {
			stats, f, t, err := memoryStore.Stats(selector, metric, from, to)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			metrics[metric] = ApiStatsData{
				From:    f,
				To:      t,
				Samples: stats.Samples,
				Avg:     stats.Avg,
				Min:     stats.Min,
				Max:     stats.Max,
			}
		}
		res = append(res, metrics)
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(res)
	if err != nil {
		log.Println(err.Error())
	}
}

func StartApiServer(address string, done chan bool) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/{from:[0-9]+}/{to:[0-9]+}/timeseries", handleTimeseries)
	r.HandleFunc("/api/{from:[0-9]+}/{to:[0-9]+}/stats", handleStats)

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
