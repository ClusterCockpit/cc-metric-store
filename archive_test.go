package main

import (
	"bufio"
	"log"
	"os"
	"testing"
	"time"
)

func TestFromCheckpoint(t *testing.T) {
	m := NewMemoryStore(map[string]MetricConfig{
		"cpi":       {Frequency: 5, Aggregation: AvgAggregation},
		"flops_any": {Frequency: 5, Aggregation: SumAggregation},
		"flops_dp":  {Frequency: 5, Aggregation: SumAggregation},
		"flops_sp":  {Frequency: 5, Aggregation: SumAggregation},
	})

	startupTime := time.Now()
	files, err := m.FromCheckpoint("./testdata/checkpoints", 1692628930)
	loadedData := m.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		t.Fatal(err)
	} else {
		log.Printf("Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
	}

	m.DebugDump(bufio.NewWriter(os.Stdout), nil)

	if files != 2 {
		t.Errorf("expected: %d, got: %d\n", 2, files)
	}
}
