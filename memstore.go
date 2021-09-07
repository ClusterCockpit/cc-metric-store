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
	lock     sync.RWMutex       //
	metrics  map[string]*buffer // Every level can store metrics.
	children map[string]*level  // Lower levels.
}

// Find the correct level for the given selector, creating it if
// it does not exist. Example selector in the context of the
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu", "0" }
// This function would probably benefit a lot from `level.children` beeing a `sync.Map`?
func (l *level) findLevelOrCreate(selector []string) *level {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	child, ok := l.children[selector[0]]
	l.lock.RUnlock()
	if ok {
		return child.findLevelOrCreate(selector[1:])
	}

	// The level does not exist, take write lock for unqiue access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	child, ok = l.children[selector[0]]
	if ok {
		l.lock.Unlock()
		return child.findLevelOrCreate(selector[1:])
	}

	child = &level{
		metrics:  make(map[string]*buffer),
		children: make(map[string]*level),
	}
	l.children[selector[0]] = child
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:])
}

// This function assmumes that `l.lock` is LOCKED!
// Read `buffer.read` for context.
// If this level does not have data for the requested metric, the data
// is aggregated timestep-wise from all the children (recursively).
func (l *level) read(metric string, from, to int64, data []Float) ([]Float, int, int64, int64, error) {
	if b, ok := l.metrics[metric]; ok {
		// Whoo, this is the "native" level of this metric:
		data, from, to, err := b.read(from, to, data)
		return data, 1, from, to, err
	}

	if len(l.children) == 0 {
		return nil, 1, 0, 0, ErrNoData
	}

	n := 0
	for _, child := range l.children {
		child.lock.RLock()
		cdata, cn, cfrom, cto, err := child.read(metric, from, to, data)
		child.lock.RUnlock()

		if err == ErrNoData {
			continue
		}

		if err != nil {
			return nil, 0, 0, 0, err
		}

		if n == 0 {
			data = cdata
			from = cfrom
			to = cto
			n += cn
			continue
		}

		if cfrom != from || cto != to {
			return nil, 0, 0, 0, ErrDataDoesNotAlign
		}

		if len(data) != len(cdata) {
			panic("WTF? Different freq. at different levels?")
		}

		n += cn
	}

	if n == 0 {
		return nil, 0, 0, 0, ErrNoData
	}

	return data, n, from, to, nil
}

func (l *level) free(t int64) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0
	for _, b := range l.metrics {
		m, err := b.free(t)
		n += m
		if err != nil {
			return n, err
		}
	}

	for _, l := range l.children {
		m, err := l.free(t)
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, nil
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
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		b, ok := l.metrics[metric.Name]
		if !ok {
			minfo, ok := m.metrics[metric.Name]
			if !ok {
				// return errors.New("unkown metric: " + metric.Name)
				continue
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

// Returns all values for metric `metric` from `from` to `to` for the selected level.
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// See `level.read` for more information.
func (m *MemoryStore) Read(selector []string, metric string, from, to int64) ([]Float, int64, int64, error) {
	l := m.root.findLevelOrCreate(selector)
	l.lock.RLock()
	defer l.lock.RUnlock()

	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	data := make([]Float, (to-from)/minfo.Frequency)
	data, n, from, to, err := l.read(metric, from, to, data)
	if err != nil {
		return nil, 0, 0, err
	}

	if n > 1 {
		if minfo.Aggregation == "avg" {
			normalize := 1. / Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if minfo.Aggregation != "sum" {
			return nil, 0, 0, errors.New("invalid aggregation strategy: " + minfo.Aggregation)
		}
	}

	return data, from, to, err
}

// Release all buffers for the selected level and all its children that contain only
// values older than `t`.
func (m *MemoryStore) Free(selector []string, t int64) (int, error) {
	return m.root.findLevelOrCreate(selector).free(t)
}
