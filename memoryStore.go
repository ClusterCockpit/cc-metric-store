package main

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/lineprotocol"
)

type storeBuffer struct {
	store []lineprotocol.Float
	start int64
}

type buffer struct {
	current *storeBuffer
	next    *storeBuffer
	lock    sync.Mutex
}

//MemoryStore holds the state for a metric memory store.
//It does not export any variable.
type MemoryStore struct {
	containers map[string]*buffer
	offsets    map[string]int
	frequency  int
	numSlots   int
	numMetrics int
	lock       sync.Mutex
}

func initBuffer(b *storeBuffer) {
	for i := 0; i < len(b.store); i++ {
		b.store[i] = lineprotocol.Float(math.NaN())
	}
}

func allocateBuffer(ts int64, size int) *buffer {
	b := new(buffer)
	s := make([]lineprotocol.Float, size)
	b.current = &storeBuffer{s, ts}
	initBuffer(b.current)

	s = make([]lineprotocol.Float, size)
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
	metrics []lineprotocol.Metric) error {

	m.lock.Lock()
	b, ok := m.containers[key]
	if !ok {
		//Key does not exist. Allocate new buffer.
		m.containers[key] = allocateBuffer(ts, m.numMetrics*m.numSlots)
		b = m.containers[key]
	}
	m.lock.Unlock()
	b.lock.Lock()
	defer b.lock.Unlock()

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
	to int64) ([]lineprotocol.Float, int64, error) {

	m.lock.Lock()
	b, ok := m.containers[key]
	m.lock.Unlock()
	if !ok {
		return nil, 0, fmt.Errorf("key %s does not exist", key)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if to <= from {
		return nil, 0, fmt.Errorf("invalid duration %d - %d", from, to)
	}

	if from > b.current.start+int64(m.numSlots*m.frequency) {
		return nil, 0, fmt.Errorf("from %d out of bounds", from)
	}

	if to < b.next.start {
		return nil, 0, fmt.Errorf("to %d out of bounds", to)
	}

	var values1, values2 []lineprotocol.Float
	offset := m.offsets[metric] * m.numSlots
	valuesFrom := from

	if from < b.current.start && b.next.start != 0 {

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

// Call *f* once on every value which *GetMetric* would
// return for similar arguments. This operation might be known
// as fold in Ruby/Haskell/Scala. It can be used to implement
// the calculation of sums, averages, minimas and maximas.
// The advantage of using this over *GetMetric* for such calculations
// is that it can be implemented without copying data.
// TODO: Write Tests, implement without calling GetMetric!
func (m *MemoryStore) Reduce(
	key string, metric string,
	from int64, to int64,
	f func(t int64, acc lineprotocol.Float, x lineprotocol.Float) lineprotocol.Float, initialX lineprotocol.Float) (lineprotocol.Float, error) {

	values, valuesFrom, err := m.GetMetric(key, metric, from, to)
	if err != nil {
		return 0.0, err
	}

	acc := initialX
	t := valuesFrom
	for i := 0; i < len(values); i++ {
		acc = f(t, acc, values[i])
		t += int64(m.frequency)
	}

	return acc, nil
}

// Return a map of keys to a map of metrics to the most recent value writen to
// the store for that metric.
// TODO: Write Tests!
func (m *MemoryStore) Peak(prefix string) map[string]map[string]lineprotocol.Float {
	m.lock.Lock()
	defer m.lock.Unlock()

	now := time.Now().Unix()

	retval := make(map[string]map[string]lineprotocol.Float)
	for key, b := range m.containers {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		b.lock.Lock()
		index := int(now-b.current.start) / m.frequency
		if index >= m.numSlots {
			index = m.numSlots - 1
		}

		vals := make(map[string]lineprotocol.Float)
		for metric, offset := range m.offsets {
			val := lineprotocol.Float(math.NaN())
			for i := index; i >= 0 && math.IsNaN(float64(val)); i -= 1 {
				val = b.current.store[offset*m.numSlots+i]
			}

			vals[metric] = val
		}

		b.lock.Unlock()
		retval[key[len(prefix):]] = vals
	}

	return retval
}
