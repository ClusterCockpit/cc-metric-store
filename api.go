package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type ApiMetricData struct {
	Error *string    `json:"error,omitempty"`
	From  int64      `json:"from"`
	To    int64      `json:"to"`
	Data  FloatArray `json:"data,omitempty"`
	Avg   Float      `json:"avg"`
	Min   Float      `json:"min"`
	Max   Float      `json:"max"`
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

func (data *ApiMetricData) PadDataWithNull(from, to int64, metric string) {
	minfo, ok := memoryStore.metrics[metric]
	if !ok {
		return
	}

	if (data.From / minfo.frequency) > (from / minfo.frequency) {
		padfront := int((data.From / minfo.frequency) - (from / minfo.frequency))
		ndata := make([]Float, 0, padfront+len(data.Data))
		for i := 0; i < padfront; i++ {
			ndata = append(ndata, NaN)
		}
		for j := 0; j < len(data.Data); j++ {
			ndata = append(ndata, data.Data[j])
		}
		data.Data = ndata
	}
}

func handleFree(rw http.ResponseWriter, r *http.Request) {
	rawTo := r.URL.Query().Get("to")
	if rawTo == "" {
		http.Error(rw, "'to' is a required query parameter", http.StatusBadRequest)
		return
	}

	to, err := strconv.ParseInt(rawTo, 10, 64)
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

func handleWrite(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	dec := lineprotocol.NewDecoderWithBytes(bytes)
	if err := decodeLine(dec); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

type ApiQueryRequest struct {
	Cluster     string     `json:"cluster"`
	From        int64      `json:"from"`
	To          int64      `json:"to"`
	WithStats   bool       `json:"with-stats"`
	WithData    bool       `json:"with-data"`
	WithPadding bool       `json:"with-padding"`
	Queries     []ApiQuery `json:"queries"`
	ForAllNodes []string   `json:"for-all-nodes"`
}

type ApiQueryResponse struct {
	Queries []ApiQuery        `json:"queries,omitempty"`
	Results [][]ApiMetricData `json:"results"`
}

type ApiQuery struct {
	Metric     string  `json:"metric"`
	Hostname   string  `json:"host"`
	Aggregate  bool    `json:"aggreg"`
	Type       *string `json:"type,omitempty"`
	TypeIds    []int   `json:"type-ids,omitempty"`
	SubType    *string `json:"subtype,omitempty"`
	SubTypeIds []int   `json:"subtype-ids,omitempty"`
}

func handleQuery(rw http.ResponseWriter, r *http.Request) {
	var err error
	var req ApiQueryRequest = ApiQueryRequest{WithStats: true, WithData: true, WithPadding: true}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	response := ApiQueryResponse{
		Results: make([][]ApiMetricData, 0, len(req.Queries)),
	}
	if req.ForAllNodes != nil {
		nodes := memoryStore.ListChildren([]string{req.Cluster})
		for _, node := range nodes {
			for _, metric := range req.ForAllNodes {
				q := ApiQuery{
					Metric:   metric,
					Hostname: node,
				}
				req.Queries = append(req.Queries, q)
				response.Queries = append(response.Queries, q)
			}
		}
	}

	for _, query := range req.Queries {
		sels := make([]Selector, 0, 1)
		if query.Aggregate || query.Type == nil {
			sel := Selector{{String: req.Cluster}, {String: query.Hostname}}
			if query.Type != nil {
				if len(query.TypeIds) == 1 {
					sel = append(sel, SelectorElement{String: fmt.Sprintf("%s%d", *query.Type, query.TypeIds[0])})
				} else {
					ids := make([]string, len(query.TypeIds))
					for i, id := range query.TypeIds {
						ids[i] = fmt.Sprintf("%s%d", *query.Type, id)
					}
					sel = append(sel, SelectorElement{Group: ids})
				}

				if query.SubType != nil {
					if len(query.SubTypeIds) == 1 {
						sel = append(sel, SelectorElement{String: fmt.Sprintf("%s%d", *query.SubType, query.SubTypeIds[0])})
					} else {
						ids := make([]string, len(query.SubTypeIds))
						for i, id := range query.SubTypeIds {
							ids[i] = fmt.Sprintf("%s%d", *query.SubType, id)
						}
						sel = append(sel, SelectorElement{Group: ids})
					}
				}
			}
			sels = append(sels, sel)
		} else {
			for _, typeId := range query.TypeIds {
				if query.SubType != nil {
					for _, subTypeId := range query.SubTypeIds {
						sels = append(sels, Selector{
							{String: req.Cluster}, {String: query.Hostname},
							{String: fmt.Sprintf("%s%d", *query.Type, typeId)},
							{String: fmt.Sprintf("%s%d", *query.SubType, subTypeId)}})
					}
				} else {
					sels = append(sels, Selector{
						{String: req.Cluster},
						{String: query.Hostname},
						{String: fmt.Sprintf("%s%d", *query.Type, typeId)}})
				}
			}
		}

		// log.Printf("query: %#v\n", query)
		// log.Printf("sels: %#v\n", sels)

		res := make([]ApiMetricData, 0, len(sels))
		for _, sel := range sels {
			data := ApiMetricData{}
			data.Data, data.From, data.To, err = memoryStore.Read(sel, query.Metric, req.From, req.To)
			// log.Printf("data: %#v, %#v, %#v, %#v", data.Data, data.From, data.To, err)
			if err != nil {
				msg := err.Error()
				data.Error = &msg
				res = append(res, data)
				continue
			}

			if req.WithStats {
				data.AddStats()
			}
			if req.WithPadding {
				data.PadDataWithNull(req.From, req.To, query.Metric)
			}
			if !req.WithData {
				data.Data = nil
			}
			res = append(res, data)
		}
		response.Results = append(response.Results, res)
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

func StartApiServer(ctx context.Context, address string, httpsConfig *HttpsConfig) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/free", handleFree)
	r.HandleFunc("/api/write", handleWrite)
	r.HandleFunc("/api/query", handleQuery)
	r.HandleFunc("/api/debug", func(rw http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(rw)
		defer bw.Flush()

		memoryStore.DebugDump(bw)
	})

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
		if httpsConfig != nil {
			log.Printf("API https endpoint listening on '%s'\n", address)
			err := server.ListenAndServeTLS(httpsConfig.CertFile, httpsConfig.KeyFile)
			if err != nil && err != http.ErrServerClosed {
				log.Println(err)
			}
		} else {
			log.Printf("API http endpoint listening on '%s'\n", address)
			err := server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Println(err)
			}
		}
	}()

	for {
		<-ctx.Done()
		err := server.Shutdown(context.Background())
		log.Println("API server shut down")
		return err
	}
}
