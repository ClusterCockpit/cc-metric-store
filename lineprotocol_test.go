package main

import (
	"log"
	"testing"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

const TestDataClassicFormat string = `
m1,cluster=ctest,hostname=htest1,type=node value=1 123456789
m2,cluster=ctest,hostname=htest1,type=node value=2 123456789
m3,cluster=ctest,hostname=htest2,type=node value=3 123456789
m4,cluster=ctest,hostname=htest2,type=core,type-id=1 value=4 123456789
m4,cluster=ctest,hostname=htest2,type-id=2,type=core value=5 123456789
`

func TestLineprotocolDecoder(t *testing.T) {
	prevMemoryStore := memoryStore
	t.Cleanup(func() {
		memoryStore = prevMemoryStore
	})

	memoryStore = NewMemoryStore(map[string]MetricConfig{
		"m1": {Frequency: 1},
		"m2": {Frequency: 1},
		"m3": {Frequency: 1},
		"m4": {Frequency: 1},
	})

	dec := lineprotocol.NewDecoderWithBytes([]byte(TestDataClassicFormat))
	if err := decodeLine(dec); err != nil {
		log.Fatal(err)
	}

	// memoryStore.DebugDump(bufio.NewWriter(os.Stderr))

	h1 := memoryStore.GetLevel([]string{"ctest", "htest1"})
	h1b1 := h1.metrics[memoryStore.metrics["m1"].offset]
	h1b2 := h1.metrics[memoryStore.metrics["m2"].offset]
	if h1b1.data[0] != 1.0 || h1b2.data[0] != 2.0 {
		log.Fatal()
	}

	h2 := memoryStore.GetLevel([]string{"ctest", "htest2"})
	h2b3 := h2.metrics[memoryStore.metrics["m3"].offset]
	if h2b3.data[0] != 3.0 {
		log.Fatal()
	}

	h2c1 := memoryStore.GetLevel([]string{"ctest", "htest2", "core1"})
	h2c1b4 := h2c1.metrics[memoryStore.metrics["m4"].offset]
	h2c2 := memoryStore.GetLevel([]string{"ctest", "htest2", "core2"})
	h2c2b4 := h2c2.metrics[memoryStore.metrics["m4"].offset]
	if h2c1b4.data[0] != 4.0 || h2c2b4.data[0] != 5.0 {
		log.Fatal()
	}
}
