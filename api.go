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
//	[
//		{ "selector": ["emmy", "host123"], "metrics": ["load_one"] }
//	]
type ApiRequestBody []struct {
	Selector []string `json:"selector"`
	Metrics  []string `json:"metrics"`
}

type ApiMetricData struct {
	From int64   `json:"from"`
	To   int64   `json:"to"`
	Data []Float `json:"data"`
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

	res := make([]map[string]ApiMetricData, 0, len(reqBody))
	for _, req := range reqBody {
		metrics := make(map[string]ApiMetricData)
		for _, metric := range req.Metrics {
			data, f, t, err := memoryStore.Read(req.Selector, metric, from, to)
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

func StartApiServer(address string, done chan bool) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/{from:[0-9]+}/{to:[0-9]+}/timeseries", handleTimeseries)

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
