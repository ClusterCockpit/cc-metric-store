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
	"math"
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
	Error *string `json:"error,omitempty"`
	From  int64   `json:"from"`
	To    int64   `json:"to"`
	Data  []Float `json:"data"`
	Avg   Float   `json:"avg"`
	Min   Float   `json:"min"`
	Max   Float   `json:"max"`
}

// TODO: Optimize this, just like the stats endpoint!
func (data *ApiMetricData) AddStats() {
	n := 0
	sum, min, max := 0.0, math.MaxFloat64, -math.MaxFloat64
	for _, x := range data.Data {
		if x.IsNaN() {
			continue
		}

		n += 1
		sum += float64(x)
		min = math.Min(min, float64(x))
		max = math.Max(max, float64(x))
	}

	if n > 0 {
		avg := sum / float64(n)
		data.Avg = Float(avg)
		data.Min = Float(min)
		data.Max = Float(max)
	} else {
		data.Avg, data.Min, data.Max = NaN, NaN, NaN
	}
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

	withStats := r.URL.Query().Get("with-stats") == "true"

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
				metrics[metric] = ApiMetricData{Error: &msg}
				continue
			}

			amd := ApiMetricData{
				From: f,
				To:   t,
				Data: data,
			}
			if withStats {
				amd.AddStats()
			}
			metrics[metric] = amd
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
				metrics[metric] = ApiStatsData{Error: &msg}
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

	dec := lineprotocol.NewDecoder(bufio.NewReader(r.Body))
	// Unlike the name suggests, handleLine can handle multiple lines
	if err := handleLine(dec); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func handleAllNodes(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterId := vars["cluster"]
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
	var reqBody struct {
		Metrics []string `json:"metrics"`
	}
	err = bodyDec.Decode(&reqBody)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	res := make(map[string]map[string]ApiMetricData)

	memoryStore.root.lock.RLock()
	cluster, ok := memoryStore.root.children[clusterId]
	memoryStore.root.lock.RUnlock()
	if !ok {
		http.Error(rw, fmt.Sprintf("cluster '%s' does not exist", clusterId), http.StatusBadRequest)
		return
	}

	cluster.lock.RLock()
	hosts := make([]string, 0, len(cluster.children))
	for host := range cluster.children {
		hosts = append(hosts, host)
	}
	cluster.lock.RUnlock()

	for _, host := range hosts {
		metrics := make(map[string]ApiMetricData)
		for _, metric := range reqBody.Metrics {
			data, f, t, err := memoryStore.Read(Selector{SelectorElement{String: clusterId}, SelectorElement{String: host}}, metric, from, to)
			if err != nil {
				// http.Error(rw, err.Error(), http.StatusInternalServerError)
				msg := err.Error()
				metrics[metric] = ApiMetricData{Error: &msg}
				continue
			}

			metrics[metric] = ApiMetricData{
				From: f,
				To:   t,
				Data: data,
			}
		}
		res[host] = metrics
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(res)
	if err != nil {
		log.Println(err.Error())
	}
}

type ApiQueryRequest struct {
	Cluster string     `json:"cluster"`
	From    int64      `json:"from"`
	To      int64      `json:"to"`
	Queries []ApiQuery `json:"queries"`
}

type ApiQueryResponse struct {
	ApiMetricData
	Query *ApiQuery `json:"query"`
}

type ApiQuery struct {
	Metric     string   `json:"metric"`
	Hostname   string   `json:"hostname"`
	Type       *string  `json:"type,omitempty"`
	TypeIds    []string `json:"type-ids,omitempty"`
	SubType    *string  `json:"subtype,omitempty"`
	SubTypeIds []string `json:"subtype-ids,omitempty"`
}

func handleQuery(rw http.ResponseWriter, r *http.Request) {
	var err error
	var req ApiQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	response := make([]ApiQueryResponse, 0, len(req.Queries))
	for _, query := range req.Queries {
		q := query // One of the few shitty things about Go: range looped-over variables are in the outer scope
		res := ApiQueryResponse{
			Query: &q,
		}

		sel := Selector{SelectorElement{String: req.Cluster}, SelectorElement{String: query.Hostname}}
		if query.Type != nil {
			if len(query.TypeIds) == 1 {
				sel = append(sel, SelectorElement{String: *query.Type + query.TypeIds[0]})
			} else {
				ids := make([]string, len(query.TypeIds))
				for i, id := range query.TypeIds {
					ids[i] = *query.Type + id
				}
				sel = append(sel, SelectorElement{Group: ids})
			}

			if query.SubType != nil {
				if len(query.SubTypeIds) == 1 {
					sel = append(sel, SelectorElement{String: *query.SubType + query.SubTypeIds[0]})
				} else {
					ids := make([]string, len(query.SubTypeIds))
					for i, id := range query.SubTypeIds {
						ids[i] = *query.SubType + id
					}
					sel = append(sel, SelectorElement{Group: ids})
				}
			}
		}

		// log.Printf("selector (metric: %s): %v", query.Metric, sel)
		res.Data, res.From, res.To, err = memoryStore.Read(sel, query.Metric, req.From, req.To)
		if err != nil {
			msg := err.Error()
			res.Error = &msg
			response = append(response, res)
			continue
		}

		res.AddStats()
		response = append(response, res)
	}

	rw.Header().Set("Content-Type", "application/json")
	bw := bufio.NewWriter(rw)
	defer bw.Flush()
	if err := json.NewEncoder(bw).Encode(response); err != nil {
		log.Print(err)
		return
	}
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
	r.HandleFunc("/api/{cluster}/{from:[0-9]+}/{to:[0-9]+}/all-nodes", handleAllNodes)
	r.HandleFunc("/api/write", handleWrite)
	r.HandleFunc("/api/query", handleQuery)

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
