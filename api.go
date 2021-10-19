package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

// Example:
//	{
//		"metrics": ["flops_sp", "flops_dp"]
//		"selectors": [["emmy", "host123", "cpu", "0"], ["emmy", "host123", "cpu", "1"]]
//	}
type ApiRequestBody struct {
	Metrics   []string   `json:"metrics"`
	Selectors []Selector `json:"selectors"`
}

type ApiMetricData struct {
	Error *string `json:"error"`
	From  int64   `json:"from"`
	To    int64   `json:"to"`
	Data  []Float `json:"data"`
}

type ApiStatsData struct {
	Error   *string `json:"error"`
	From    int64   `json:"from"`
	To      int64   `json:"to"`
	Samples int     `json:"samples"`
	Avg     Float   `json:"avg"`
	Min     Float   `json:"min"`
	Max     Float   `json:"max"`
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
				// http.Error(rw, err.Error(), http.StatusInternalServerError)
				msg := err.Error()
				metrics[metric] = ApiMetricData{ Error: &msg }
				continue
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
				// http.Error(rw, err.Error(), http.StatusInternalServerError)
				msg := err.Error()
				metrics[metric] = ApiStatsData{ Error: &msg }
				continue
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

func handleFree(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	to, err := strconv.ParseInt(vars["to"], 10, 64)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: lastCheckpoint might be modified by different go-routines.
	// Load it using the sync/atomic package?
	freeUpTo := lastCheckpoint.Unix()
	if to < freeUpTo {
		freeUpTo = to
	}

	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyDec := json.NewDecoder(r.Body)
	var selectors []Selector
	err = bodyDec.Decode(&selectors)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	n := 0
	for _, sel := range selectors {
		bn, err := memoryStore.Free(sel, freeUpTo)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		n += bn
	}

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(fmt.Sprintf("buffers freed: %d\n", n)))
}

func handlePeek(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cluster := vars["cluster"]
	res, err := memoryStore.Peek(cluster)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(res)
	if err != nil {
		log.Println(err.Error())
	}
}

func handleWrite(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	reader := bufio.NewReader(r.Body)
	dec := lineprotocol.NewDecoder(reader)
	// Unlike the name suggests, handleLine can handle multiple lines
	if err := handleLine(dec); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func authentication(next http.Handler, publicKey ed25519.PublicKey) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		authheader := r.Header.Get("Authorization")
		if authheader == "" || !strings.HasPrefix(authheader, "Bearer ") {
			http.Error(rw, "Use JWT Authentication", http.StatusUnauthorized)
			return
		}

		// The actual token is ignored for now.
		// In case expiration and so on are specified, the Parse function
		// already returns an error for expired tokens.
		_, err := jwt.Parse(authheader[len("Bearer "):], func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodEdDSA {
				return nil, errors.New("only Ed25519/EdDSA supported")
			}

			return publicKey, nil
		})

		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		// Let request through...
		next.ServeHTTP(rw, r)
	})
}

func StartApiServer(address string, ctx context.Context) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/{from:[0-9]+}/{to:[0-9]+}/timeseries", handleTimeseries)
	r.HandleFunc("/api/{from:[0-9]+}/{to:[0-9]+}/stats", handleStats)
	r.HandleFunc("/api/{to:[0-9]+}/free", handleFree)
	r.HandleFunc("/api/{cluster}/peek", handlePeek)
	r.HandleFunc("/api/write", handleWrite)

	server := &http.Server{
		Handler:      r,
		Addr:         address,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	if len(conf.JwtPublicKey) > 0 {
		buf, err := base64.StdEncoding.DecodeString(conf.JwtPublicKey)
		if err != nil {
			return err
		}
		publicKey := ed25519.PublicKey(buf)
		server.Handler = authentication(server.Handler, publicKey)
	}

	go func() {
		log.Printf("API http endpoint listening on '%s'\n", address)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Println(err)
		}
	}()

	for {
		<-ctx.Done()
		err := server.Shutdown(context.Background())
		log.Println("API server shut down")
		return err
	}
}
