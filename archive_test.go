package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveReadDir(t *testing.T) {
	memoryStore = NewMemoryStore(map[string]MetricConfig{
		"flops_dp": {Frequency: 5},
		"flops_sp": {Frequency: 5},
	})
	data := `{"from":1692628930,"to":1692629003,"metrics":{},"children":{"hwthread0":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[2.2,0.7,2.1,1.6,2.3,0.7,0.9,1.8,0.7,1.7,1.5,1.8,0.9,1.6]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.3,0.0,0.0,0.0,0.0,28.2,5.1,4.7,0.1,0.3,0.0,0.2,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.5,0.1,0.0,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.3,0.0,0.0,0.0,0.0,27.2,4.8,4.7,0.0,0.3,0.0,0.2,0.0,0.0]}},"children":{}},"hwthread1":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[0.9,4.2,4.2,3.8,3.1,0.7,1.3,0.7,1.8,1.6,5.0,3.0,4.9,5.2]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.5,0.0,0.0,0.0,0.0,49.8,5.0,0.1,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.4,0.1,0.0,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.5,0.0,0.0,0.0,0.0,49.1,4.9,0.1,0.0,0.0,0.0,0.0,0.0,0.0]}},"children":{}},"hwthread2":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[1.8,0.9,2.2,2.1,4.3,1.4,0.8,1.0,1.1,1.6,3.9,4.9,2.7,5.1]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,16.2,4.4,0.0,0.1,0.0,0.0,0.0,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.1,0.2,0.0,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,16.1,3.9,0.0,0.1,0.0,0.0,0.0,0.0,0.0]}},"children":{}},"hwthread3":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[2.0,0.6,6.0,3.5,2.3,0.9,1.2,1.2,1.1,1.0,2.0,0.7,1.1,3.6]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,1.6,11.5,0.0,0.0,0.0,0.0,0.2,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.1,0.4,0.0,0.0,0.0,0.0,0.1,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,1.4,10.6,0.0,0.0,0.0,0.0,0.0,0.0,0.0]}},"children":{}},"hwthread4":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[1.0,1.5,1.3,0.9,0.8,0.8,1.2,1.2,1.4,1.0,0.8,2.3,2.7,0.8]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.8,0.0,0.0,0.0,0.0,3.4,1.1,2.2,0.4,0.0,0.0,0.0,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.7,0.3,0.2,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.8,0.0,0.0,0.0,0.0,2.0,0.5,1.8,0.4,0.0,0.0,0.0,0.0,0.0]}},"children":{}},"hwthread5":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[0.9,2.3,1.1,0.7,0.8,1.5,1.0,0.9,0.9,1.0,0.8,1.1,1.0,2.0]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.5,0.0,1.9,0.1,0.3,3.0,13.8,0.0,1.7,1.1,0.0,0.0,0.5,1.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.3,0.0,0.1,0.0,0.2,0.1,0.9,0.0,0.8,0.6,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,1.7,0.0,0.0,2.9,12.0,0.0,0.0,0.0,0.0,0.0,0.5,1.0]}},"children":{}},"hwthread6":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[1.4,0.8,0.8,1.2,1.4,0.8,1.2,1.1,1.5,0.9,1.5,1.5,1.4,0.8]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.1,0.0,3.9,10.6,3.0,1.4,7.0,0.0,1.0,0.2,0.0,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.1,0.0,0.0,0.0,0.1,0.1,0.2,0.0,0.4,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,3.9,10.6,2.9,1.2,6.6,0.0,0.1,0.2,0.0,0.0,0.0]}},"children":{}},"hwthread7":{"from":1692628930,"to":1692629003,"metrics":{"cpi":{"frequency":5,"start":1692628930,"unit":"","data":[1.2,1.2,1.3,0.8,1.2,1.5,1.0,0.7,1.7,1.8,3.5,0.7,1.5,4.6]},"flops_any":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.3,0.0,2.3,4.7,0.0,12.5,0.7,0.2,0.2,0.0,0.0]},"flops_dp":{"frequency":5,"start":1692628931,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.0,0.0,0.1,0.9,0.0,0.0,0.0,0.0,0.0,0.0,0.0]},"flops_sp":{"frequency":5,"start":1692628930,"unit":"MFLOP/s","data":[0.0,0.0,0.0,0.3,0.0,2.2,2.8,0.0,12.5,0.7,0.2,0.2,0.0,0.0]}},"children":{}}}}`

	tmpdir := t.TempDir()
	folder := filepath.Join(tmpdir, "testcluster", "nuc")
	filename := filepath.Join(folder, "1692628930.json")

	err := os.MkdirAll(folder, 0755)
	if err != nil {
		t.Error(err)
	}
	f, err := os.Create(filename)
	if err != nil {
		t.Error(err)
	}
	f.WriteString(data)
	f.Close()

	startupTime := time.Unix(1692628930, 0)
	_, err = memoryStore.FromCheckpoint(tmpdir, startupTime.Unix())
	if err != nil {
		t.Error(err)
		return
	}

	querydata := ApiMetricData{}
	querysel := Selector{
		{String: "testcluster"},
		{String: "nuc"},
		{String: "hwthread0"}}

	querydata.Data, querydata.From, querydata.To, querydata.Unit, err = memoryStore.Read(querysel, "flops_dp", 1692628930, time.Now().Unix())
	if err != nil {
		t.Error(err)
		return
	}
	if querydata.Unit != "MFLOP/s" {
		t.Errorf("Metric flops_dp does not have the right unit")
		return
	}
}
