package api

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/ClusterCockpit/cc-metric-store/internal/memorystore"
	"github.com/ClusterCockpit/cc-metric-store/internal/util"
	"github.com/gorilla/mux"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

type ApiMetricData struct {
	Error *string         `json:"error,omitempty"`
	Data  util.FloatArray `json:"data,omitempty"`
	From  int64           `json:"from"`
	To    int64           `json:"to"`
	Avg   util.Float      `json:"avg"`
	Min   util.Float      `json:"min"`
	Max   util.Float      `json:"max"`
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
		data.Avg = util.Float(avg)
		data.Min = util.Float(min)
		data.Max = util.Float(max)
	} else {
		data.Avg, data.Min, data.Max = util.NaN, util.NaN, util.NaN
	}
}

func (data *ApiMetricData) ScaleBy(f util.Float) {
	if f == 0 || f == 1 {
		return
	}

	data.Avg *= f
	data.Min *= f
	data.Max *= f
	for i := 0; i < len(data.Data); i++ {
		data.Data[i] *= f
	}
}

func (data *ApiMetricData) PadDataWithNull(ms *memorystore.MemoryStore, from, to int64, metric string) {
	minfo, ok := ms.Metrics[metric]
	if !ok {
		return
	}

	if (data.From / minfo.Frequency) > (from / minfo.Frequency) {
		padfront := int((data.From / minfo.Frequency) - (from / minfo.Frequency))
		ndata := make([]util.Float, 0, padfront+len(data.Data))
		for i := 0; i < padfront; i++ {
			ndata = append(ndata, util.NaN)
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

	// // TODO: lastCheckpoint might be modified by different go-routines.
	// // Load it using the sync/atomic package?
	// freeUpTo := lastCheckpoint.Unix()
	// if to < freeUpTo {
	// 	freeUpTo = to
	// }

	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyDec := json.NewDecoder(r.Body)
	var selectors [][]string
	err = bodyDec.Decode(&selectors)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	ms := memorystore.GetMemoryStore()
	n := 0
	for _, sel := range selectors {
		bn, err := ms.Free(sel, to)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		n += bn
	}

	rw.WriteHeader(http.StatusOK)
	fmt.Fprintf(rw, "buffers freed: %d\n", n)
}

func handleWrite(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error while reading request body: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	ms := memorystore.GetMemoryStore()
	dec := lineprotocol.NewDecoderWithBytes(bytes)
	if err := decodeLine(dec, ms, r.URL.Query().Get("cluster")); err != nil {
		log.Printf("/api/write error: %s", err.Error())
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	rw.WriteHeader(http.StatusOK)
}

type ApiQueryRequest struct {
	Cluster     string     `json:"cluster"`
	Queries     []ApiQuery `json:"queries"`
	ForAllNodes []string   `json:"for-all-nodes"`
	From        int64      `json:"from"`
	To          int64      `json:"to"`
	WithStats   bool       `json:"with-stats"`
	WithData    bool       `json:"with-data"`
	WithPadding bool       `json:"with-padding"`
}

type ApiQueryResponse struct {
	Queries []ApiQuery        `json:"queries,omitempty"`
	Results [][]ApiMetricData `json:"results"`
}

type ApiQuery struct {
	Type        *string    `json:"type,omitempty"`
	SubType     *string    `json:"subtype,omitempty"`
	Metric      string     `json:"metric"`
	Hostname    string     `json:"host"`
	TypeIds     []string   `json:"type-ids,omitempty"`
	SubTypeIds  []string   `json:"subtype-ids,omitempty"`
	ScaleFactor util.Float `json:"scale-by,omitempty"`
	Aggregate   bool       `json:"aggreg"`
}

func handleQuery(rw http.ResponseWriter, r *http.Request) {
	var err error
	req := ApiQueryRequest{WithStats: true, WithData: true, WithPadding: true}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	ms := memorystore.GetMemoryStore()

	response := ApiQueryResponse{
		Results: make([][]ApiMetricData, 0, len(req.Queries)),
	}
	if req.ForAllNodes != nil {
		nodes := ms.ListChildren([]string{req.Cluster})
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
		sels := make([]util.Selector, 0, 1)
		if query.Aggregate || query.Type == nil {
			sel := util.Selector{{String: req.Cluster}, {String: query.Hostname}}
			if query.Type != nil {
				if len(query.TypeIds) == 1 {
					sel = append(sel, util.SelectorElement{String: *query.Type + query.TypeIds[0]})
				} else {
					ids := make([]string, len(query.TypeIds))
					for i, id := range query.TypeIds {
						ids[i] = *query.Type + id
					}
					sel = append(sel, util.SelectorElement{Group: ids})
				}

				if query.SubType != nil {
					if len(query.SubTypeIds) == 1 {
						sel = append(sel, util.SelectorElement{String: *query.SubType + query.SubTypeIds[0]})
					} else {
						ids := make([]string, len(query.SubTypeIds))
						for i, id := range query.SubTypeIds {
							ids[i] = *query.SubType + id
						}
						sel = append(sel, util.SelectorElement{Group: ids})
					}
				}
			}
			sels = append(sels, sel)
		} else {
			for _, typeId := range query.TypeIds {
				if query.SubType != nil {
					for _, subTypeId := range query.SubTypeIds {
						sels = append(sels, util.Selector{
							{String: req.Cluster},
							{String: query.Hostname},
							{String: *query.Type + typeId},
							{String: *query.SubType + subTypeId},
						})
					}
				} else {
					sels = append(sels, util.Selector{
						{String: req.Cluster},
						{String: query.Hostname},
						{String: *query.Type + typeId},
					})
				}
			}
		}

		// log.Printf("query: %#v\n", query)
		// log.Printf("sels: %#v\n", sels)

		res := make([]ApiMetricData, 0, len(sels))
		for _, sel := range sels {
			data := ApiMetricData{}
			data.Data, data.From, data.To, err = ms.Read(sel, query.Metric, req.From, req.To)
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
			if query.ScaleFactor != 0 {
				data.ScaleBy(query.ScaleFactor)
			}
			if req.WithPadding {
				data.PadDataWithNull(ms, req.From, req.To, query.Metric)
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

func handleDebug(rw http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("selector")
	selector := []string{}
	if len(raw) != 0 {
		selector = strings.Split(raw, ":")
	}

	ms := memorystore.GetMemoryStore()
	if err := ms.DebugDump(bufio.NewWriter(rw), selector); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte(err.Error()))
	}
}

func StartApiServer(ctx context.Context, httpConfig *config.HttpConfig) error {
	r := mux.NewRouter()

	r.HandleFunc("/api/free", handleFree)
	r.HandleFunc("/api/write", handleWrite)
	r.HandleFunc("/api/query", handleQuery)
	r.HandleFunc("/api/debug", handleDebug)

	server := &http.Server{
		Handler:      r,
		Addr:         httpConfig.Address,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	if len(config.Keys.JwtPublicKey) > 0 {
		buf, err := base64.StdEncoding.DecodeString(config.Keys.JwtPublicKey)
		if err != nil {
			return err
		}
		publicKey := ed25519.PublicKey(buf)
		server.Handler = authentication(server.Handler, publicKey)
	}

	go func() {
		if httpConfig.CertFile != "" && httpConfig.KeyFile != "" {
			log.Printf("API https endpoint listening on '%s'\n", httpConfig.Address)
			err := server.ListenAndServeTLS(httpConfig.CertFile, httpConfig.KeyFile)
			if err != nil && err != http.ErrServerClosed {
				log.Println(err)
			}
		} else {
			log.Printf("API http endpoint listening on '%s'\n", httpConfig.Address)
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
