package main

import (
	"errors"
	"sync"
)

// Default buffer capacity.
// `buffer.data` will only ever grow up to it's capacity and a new link
// in the buffer chain will be created if needed so that no copying
// of needs to happen on writes.
const (
	BUFFER_CAP int = 1024
)

// So that we can reuse allocations
var bufferPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return make([]Float, 0, BUFFER_CAP)
	},
}

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
func (b *buffer) read(from, to int64) ([]Float, int64, int64, error) {
	if from < b.start {
		if b.prev != nil {
			return b.prev.read(from, to)
		}
		from = b.start
	}

	data := make([]Float, 0, (to-from)/b.frequency+1)

	var t int64
	for t = from; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			b = b.next
			if b == nil {
				return data, from, t, nil
			}
			idx = 0
		}

		if t < b.start || idx >= len(b.data) {
			data = append(data, NaN)
		} else {
			data = append(data, b.data[idx])
		}
	}

	return data, from, t, nil
}

// Could also be called "node" as this forms a node in a tree structure.
// Called level because "node" might be confusing here.
// Can be both a leaf or a inner node. In this tree structue, inner nodes can
// also hold data (in `metrics`).
type level struct {
	lock     sync.Mutex         // There is performance to be gained by having different locks for `metrics` and `children` (Spinlock?).
	metrics  map[string]*buffer // Every level can store metrics.
	children map[string]*level  // Sub-granularities/nodes. Use `sync.Map`?
}

// Caution: the lock of the returned level will be LOCKED.
// Find the correct level for the given selector, creating it if
// it does not exist. Example selector in the context of the
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu", "0" }
// This function would probably benefit a lot from `level.children` beeing a `sync.Map`?
func (l *level) findLevelOrCreate(selector []string) *level {
	l.lock.Lock()
	if len(selector) == 0 {
		return l
	}

	child, ok := l.children[selector[0]]
	if !ok {
		child = &level{
			metrics:  make(map[string]*buffer),
			children: make(map[string]*level),
		}
		l.children[selector[0]] = child
	}

	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:])
}

// This function assmumes that `l.lock` is LOCKED!
// Read `buffer.read` for context. This function does
// a lot of short-lived allocations and copies if this is
// not the "native" level for the requested metric. There
// is a lot of optimization potential here!
// If this level does not have data for the requested metric, the data
// is aggregated timestep-wise from all the children (recursively).
// Optimization suggestion: Pass a buffer as argument onto which the values should be added.
func (l *level) read(metric string, from, to int64, aggregation string) ([]Float, int64, int64, error) {
	if b, ok := l.metrics[metric]; ok {
		// Whoo, this is the "native" level of this metric:
		return b.read(from, to)
	}

	if len(l.children) == 0 {
		return nil, 0, 0, errors.New("no data for that metric/level")
	}

	if len(l.children) == 1 {
		for _, child := range l.children {
			child.lock.Lock()
			data, from, to, err := child.read(metric, from, to, aggregation)
			child.lock.Unlock()
			return data, from, to, err
		}
	}

	// "slow" case: We need to accumulate metrics accross levels/scopes/tags/whatever.
	var data []Float = nil
	for _, child := range l.children {
		child.lock.Lock()
		cdata, cfrom, cto, err := child.read(metric, from, to, aggregation)
		child.lock.Unlock()

		if err != nil {
			return nil, 0, 0, err
		}

		if data == nil {
			data = cdata
			from = cfrom
			to = cto
			continue
		}

		if cfrom != from || cto != to {
			// TODO: Here, we could take the max of cfrom and from and the min of cto and to instead.
			// This would mean that we also have to resize data.
			return nil, 0, 0, errors.New("data for metrics at child levels does not align")
		}

		if len(data) != len(cdata) {
			panic("WTF? Different freq. at different levels?")
		}

		for i := 0; i < len(data); i++ {
			data[i] += cdata[i]
		}
	}

	switch aggregation {
	case "sum":
		return data, from, to, nil
	case "avg":
		normalize := 1. / Float(len(l.children))
		for i := 0; i < len(data); i++ {
			data[i] *= normalize
		}
		return data, from, to, nil
	default:
		return nil, 0, 0, errors.New("invalid aggregation strategy: " + aggregation)
	}
}

type MemoryStore struct {
	root    level // root of the tree structure
	metrics map[string]MetricConfig
}

func NewMemoryStore(metrics map[string]MetricConfig) *MemoryStore {
	return &MemoryStore{
		root: level{
			metrics:  make(map[string]*buffer),
			children: make(map[string]*level),
		},
		metrics: metrics,
	}
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	l := m.root.findLevelOrCreate(selector)
	defer l.lock.Unlock()

	for _, metric := range metrics {
		b, ok := l.metrics[metric.Name]
		if !ok {
			minfo, ok := m.metrics[metric.Name]
			if !ok {
				return errors.New("unkown metric: " + metric.Name)
			}

			// First write to this metric and level
			b = newBuffer(ts, minfo.Frequency)
			l.metrics[metric.Name] = b
		}

		nb, err := b.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write created a new buffer...
		if b != nb {
			l.metrics[metric.Name] = nb
		}
	}
	return nil
}

func (m *MemoryStore) Read(selector []string, metric string, from, to int64) ([]Float, int64, int64, error) {
	l := m.root.findLevelOrCreate(selector)
	defer l.lock.Unlock()

	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	return l.read(metric, from, to, minfo.Aggregation)
}
