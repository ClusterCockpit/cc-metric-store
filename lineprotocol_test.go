package main

import (
	"bytes"
	"log"
	"strconv"
	"testing"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

const TestDataClassicFormat string = `
m1,cluster=ctest,hostname=htest1,type=node value=1 123456789
m2,cluster=ctest,hostname=htest1,type=node value=2 123456789
m3,hostname=htest2,type=node value=3 123456789
m4,cluster=ctest,hostname=htest2,type=core,type-id=1 value=4 123456789
m4,cluster=ctest,hostname=htest2,type-id=2,type=core value=5 123456789
`

const BenchmarkLineBatch string = `
nm1,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm2,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm3,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm4,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm5,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm6,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm7,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm8,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
nm9,cluster=ctest,hostname=htest1,type=node value=123.0 123456789
cm1,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm2,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm3,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm4,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm5,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm6,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm7,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm8,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm9,cluster=ctest,hostname=htest1,type=core,type-id=1 value=234.0 123456789
cm1,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm2,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm3,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm4,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm5,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm6,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm7,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm8,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm9,cluster=ctest,hostname=htest1,type=core,type-id=2 value=345.0 123456789
cm1,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm2,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm3,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm4,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm5,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm6,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm7,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm8,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm9,cluster=ctest,hostname=htest1,type=core,type-id=3 value=456.0 123456789
cm1,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm2,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm3,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm4,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm5,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm6,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm7,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm8,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
cm9,cluster=ctest,hostname=htest1,type=core,type-id=4 value=567.0 123456789
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
	if err := decodeLine(dec, "ctest"); err != nil {
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

func BenchmarkLineprotocolDecoder(b *testing.B) {
	b.StopTimer()
	memoryStore = NewMemoryStore(map[string]MetricConfig{
		"nm1": {Frequency: 1},
		"nm2": {Frequency: 1},
		"nm3": {Frequency: 1},
		"nm4": {Frequency: 1},
		"nm5": {Frequency: 1},
		"nm6": {Frequency: 1},
		"nm7": {Frequency: 1},
		"nm8": {Frequency: 1},
		"nm9": {Frequency: 1},
		"cm1": {Frequency: 1},
		"cm2": {Frequency: 1},
		"cm3": {Frequency: 1},
		"cm4": {Frequency: 1},
		"cm5": {Frequency: 1},
		"cm6": {Frequency: 1},
		"cm7": {Frequency: 1},
		"cm8": {Frequency: 1},
		"cm9": {Frequency: 1},
	})

	for i := 0; i < b.N; i++ {
		data := []byte(BenchmarkLineBatch)
		data = bytes.ReplaceAll(data, []byte("123456789"), []byte(strconv.Itoa(i+123456789)))
		dec := lineprotocol.NewDecoderWithBytes(data)

		b.StartTimer()
		if err := decodeLine(dec, "ctest"); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
	}
}
