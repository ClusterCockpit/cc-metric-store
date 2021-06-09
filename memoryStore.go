package main

import (
	"fmt"
	"math"
)

type storeBuffer struct {
	store []float64
	start int64
}

type buffer struct {
	current *storeBuffer
	next    *storeBuffer
}

//MemoryStore holds the state for a metric memory store.
//It does not export any variable.
type MemoryStore struct {
	containers map[string]*buffer
	offsets    map[string]int
	frequency  int
	numSlots   int
	numMetrics int
}

func initBuffer(b *storeBuffer) {
	for i := 0; i < len(b.store); i++ {
		b.store[i] = math.NaN()
	}
}

func allocateBuffer(ts int64, size int) *buffer {
	b := new(buffer)
	s := make([]float64, size)
	b.current = &storeBuffer{s, ts}
	initBuffer(b.current)

	s = make([]float64, size)
	b.next = &storeBuffer{s, 0}
	initBuffer(b.next)
	return b
}

func switchBuffers(m *MemoryStore, b *buffer) {
	initBuffer(b.next)
	b.current, b.next = b.next, b.current
	b.current.start = b.next.start + int64(m.numSlots*m.frequency)
}

func newMemoryStore(o []string, n int, f int) *MemoryStore {
	var m MemoryStore
	m.frequency = f
	m.numSlots = n
	m.containers = make(map[string]*buffer)
	m.offsets = make(map[string]int)

	for i, name := range o {
		m.offsets[name] = i
	}

	m.numMetrics = len(o)

	return &m
}

// AddMetrics writes metrics to the memoryStore for entity key
// at unix epoch time ts. The unit of ts is seconds.
// An error is returned if ts is out of bounds of MemoryStore.
func (m *MemoryStore) AddMetrics(
	key string,
	ts int64,
	metrics []Metric) error {

	b, ok := m.containers[key]

	if !ok {
		//Key does not exist. Allocate new buffer.
		m.containers[key] = allocateBuffer(ts, m.numMetrics*m.numSlots)
		b = m.containers[key]
	}

	index := int(ts-b.current.start) / m.frequency

	if index < 0 || index >= 2*m.numSlots {
		return fmt.Errorf("ts %d out of bounds", ts)
	}

	if index >= m.numSlots {
		//Index exceeds buffer length. Switch buffers.
		switchBuffers(m, b)
		index = int(ts-b.current.start) / m.frequency
	}

	s := b.current.store

	for _, metric := range metrics {
		s[m.offsets[metric.Name]*m.numSlots+index] = metric.Value
	}

	return nil
}

// GetMetric returns a slize with metric values for timerange
// and entity key. Returns an error if key does not exist,
// stop is before start or start is in the future.
func (m *MemoryStore) GetMetric(
	key string,
	metric string,
	from int64,
	to int64) ([]float64, int64, error) {

	b, ok := m.containers[key]

	if !ok {
		return nil, 0, fmt.Errorf("key %s does not exist", key)
	}

	if to <= from {
		return nil, 0, fmt.Errorf("invalid duration %d - %d", from, to)
	}

	if from > b.current.start+int64(m.numSlots*m.frequency) {
		return nil, 0, fmt.Errorf("from %d out of bounds", from)
	}

	if to < b.next.start {
		return nil, 0, fmt.Errorf("to %d out of bounds", to)
	}

	var values1, values2 []float64
	offset := m.offsets[metric]
	valuesFrom := from

	if from < b.current.start {

		var start, stop = 0, m.numSlots

		if from > b.next.start {
			start = int(from-b.next.start) / m.frequency
		} else {
			valuesFrom = b.next.start
		}

		if to < b.current.start {
			stop = int(to-b.next.start) / m.frequency
		}

		// fmt.Println("NEXT", start, stop)
		values1 = b.next.store[offset+start : offset+stop]
	}

	if to >= b.current.start {

		var start, stop = 0, m.numSlots

		if from > b.current.start {
			start = int(from-b.current.start) / m.frequency
		}

		if to <= b.current.start+int64(m.numSlots*m.frequency) {
			stop = int(to-b.current.start) / m.frequency
		}

		// fmt.Println("CURRENT", start, stop, b.current.start)
		values2 = b.current.store[offset+start : offset+stop]
	}

	return append(values1, values2...), valuesFrom, nil
}
