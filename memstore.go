package main

import (
	"errors"
	"sync"
)

// Default buffer capacity.
// `buffer.data` will only ever grow up to it's capacity and a new link
// in the buffer chain will be created if needed so that no copying
// of data or reallocation needs to happen on writes.
const (
	BUFFER_CAP int = 1024
)

// So that we can reuse allocations
var bufferPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return make([]Float, 0, BUFFER_CAP)
	},
}

var (
	ErrNoData           error = errors.New("no data for this metric/level")
	ErrDataDoesNotAlign error = errors.New("data from lower granularities does not align")
)

// Each metric on each level has it's own buffer.
// This is where the actual values go.
// If `cap(data)` is reached, a new buffer is created and
// becomes the new head of a buffer list.
type buffer struct {
	frequency  int64   // Time between two "slots"
	start      int64   // Timestamp of when `data[0]` was written.
	data       []Float // The slice should never reallocacte as `cap(data)` is respected.
	prev, next *buffer // `prev` contains older data, `next` newer data.
}

func newBuffer(ts, freq int64) *buffer {
	return &buffer{
		frequency: freq,
		start:     ts,
		data:      bufferPool.Get().([]Float)[:0],
		prev:      nil,
		next:      nil,
	}
}

// If a new buffer was created, the new head is returnd.
// Otherwise, the existing buffer is returnd.
// Normaly, only "newer" data should be written, but if the value would
// end up in the same buffer anyways it is allowed.
func (b *buffer) write(ts int64, value Float) (*buffer, error) {
	if ts < b.start {
		return nil, errors.New("cannot write value to buffer from past")
	}

	idx := int((ts - b.start) / b.frequency)
	if idx >= cap(b.data) {
		newbuf := newBuffer(ts, b.frequency)
		newbuf.prev = b
		b.next = newbuf
		b = newbuf
		idx = 0
	}

	// Overwriting value or writing value from past
	if idx < len(b.data) {
		b.data[idx] = value
		return b, nil
	}

	// Fill up unwritten slots with NaN
	for i := len(b.data); i < idx; i++ {
		b.data = append(b.data, NaN)
	}

	b.data = append(b.data, value)
	return b, nil
}

// Return all known values from `from` to `to`. Gaps of information are
// represented by NaN. If values at the start or end are missing,
// instead of NaN values, the second and thrid return values contain
// the actual `from`/`to`.
// This function goes back the buffer chain if `from` is older than the
// currents buffer start.
// The loaded values are added to `data` and `data` is returned, possibly with a shorter length.
// If `data` is not long enough to hold all values, this function will panic!
func (b *buffer) read(from, to int64, data []Float) ([]Float, int64, int64, error) {
	if from < b.start {
		if b.prev != nil {
			return b.prev.read(from, to, data)
		}
		from = b.start
	}

	var i int = 0
	var t int64 = from
	for ; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			b = b.next
			if b == nil {
				return data, from, t, nil
			}
			idx = 0
		}

		if t < b.start || idx >= len(b.data) {
			data[i] += NaN
		} else {
			data[i] += b.data[idx]
		}
		i++
	}

	return data[:i], from, t, nil
}

// Free up and free all buffers in the chain only containing data
// older than `t`.
func (b *buffer) free(t int64) (int, error) {
	end := b.start + int64(len(b.data))*b.frequency
	if end < t && b.next != nil {
		b.next.prev = nil
		n := 0
		for b != nil {
			prev := b.prev
			if prev != nil && prev.start > b.start {
				panic("time travel?")
			}

			n += 1
			bufferPool.Put(b.data)
			b.data = nil
			b.next = nil
			b.prev = nil
			b = prev
		}
		return n, nil
	}

	if b.prev != nil {
		return b.prev.free(t)
	}

	return 0, nil
}

// Could also be called "node" as this forms a node in a tree structure.
// Called level because "node" might be confusing here.
// Can be both a leaf or a inner node. In this tree structue, inner nodes can
// also hold data (in `metrics`).
type level struct {
	lock     sync.RWMutex
	metrics  []*buffer         // Every level can store metrics.
	children map[string]*level // Lower levels.
}

// Find the correct level for the given selector, creating it if
// it does not exist. Example selector in the context of the
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu", "0" }
// This function would probably benefit a lot from `level.children` beeing a `sync.Map`?
func (l *level) findLevelOrCreate(selector []string, nMetrics int) *level {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	var child *level
	var ok bool
	if l.children == nil {
		// Children map needs to be created...
		l.lock.RUnlock()
	} else {
		child, ok := l.children[selector[0]]
		l.lock.RUnlock()
		if ok {
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	}

	// The level does not exist, take write lock for unqiue access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	if l.children != nil {
		child, ok = l.children[selector[0]]
		if ok {
			l.lock.Unlock()
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	} else {
		l.children = make(map[string]*level)
	}

	child = &level{
		metrics:  make([]*buffer, nMetrics),
		children: nil,
	}

	l.children[selector[0]] = child
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:], nMetrics)
}

type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

type MemoryStore struct {
	root    level // root of the tree structure
	metrics map[string]struct {
		offset      int
		aggregation AggregationStrategy
		frequency   int64
	}
}

func NewMemoryStore(metrics map[string]MetricConfig) *MemoryStore {
	ms := make(map[string]struct {
		offset      int
		aggregation AggregationStrategy
		frequency   int64
	})

	offset := 0
	for key, config := range metrics {
		aggregation := NoAggregation
		if config.Aggregation == "sum" {
			aggregation = SumAggregation
		} else if config.Aggregation == "avg" {
			aggregation = AvgAggregation
		} else if config.Aggregation != "" {
			panic("invalid aggregation strategy: " + config.Aggregation)
		}

		ms[key] = struct {
			offset      int
			aggregation AggregationStrategy
			frequency   int64
		}{
			offset:      offset,
			aggregation: aggregation,
			frequency:   config.Frequency,
		}

		offset += 1
	}

	return &MemoryStore{
		root: level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*level),
		},
		metrics: ms,
	}
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	l := m.root.findLevelOrCreate(selector, len(m.metrics))
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		minfo, ok := m.metrics[metric.Name]
		if !ok {
			continue
		}

		b := l.metrics[minfo.offset]
		if b == nil {
			// First write to this metric and level
			b = newBuffer(ts, minfo.frequency)
			l.metrics[minfo.offset] = b
		}

		nb, err := b.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write created a new buffer...
		if b != nb {
			l.metrics[minfo.offset] = nb
		}
	}
	return nil
}

// Returns all values for metric `metric` from `from` to `to` for the selected level.
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// See `level.read` for more information.
func (m *MemoryStore) Read(selector Selector, metric string, from, to int64) ([]Float, int64, int64, error) {
	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, data := 0, make([]Float, (to-from)/minfo.frequency+1)
	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer) error {
		cdata, cfrom, cto, err := b.read(from, to, data)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto || len(data) != len(cdata) {
			return ErrDataDoesNotAlign
		}

		data = cdata
		n += 1
		return nil
	})

	if err != nil {
		return nil, 0, 0, err
	} else if n == 0 {
		return nil, 0, 0, errors.New("metric not found")
	} else if n > 1 {
		if minfo.aggregation == AvgAggregation {
			normalize := 1. / Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if minfo.aggregation != SumAggregation {
			return nil, 0, 0, errors.New("invalid aggregation")
		}
	}

	return data, from, to, nil
}

// Release all buffers for the selected level and all its children that contain only
// values older than `t`.
func (m *MemoryStore) Free(selector Selector, t int64) (int, error) {
	n := 0
	err := m.root.findBuffers(selector, -1, func(b *buffer) error {
		m, err := b.free(t)
		n += m
		return err
	})
	return n, err
}