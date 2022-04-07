package main

import (
	"errors"
	"sync"
	"unsafe"
)

// Default buffer capacity.
// `buffer.data` will only ever grow up to it's capacity and a new link
// in the buffer chain will be created if needed so that no copying
// of data or reallocation needs to happen on writes.
const (
	BUFFER_CAP int = 512
)

// So that we can reuse allocations
var bufferPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return &buffer{
			data: make([]Float, 0, BUFFER_CAP),
		}
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
	archived   bool    // If true, this buffer is already archived

	closed bool
	/*
		statisticts struct {
			samples int
			min     Float
			max     Float
			avg     Float
		}
	*/
}

func newBuffer(ts, freq int64) *buffer {
	b := bufferPool.Get().(*buffer)
	b.frequency = freq
	b.start = ts - (freq / 2)
	b.prev = nil
	b.next = nil
	b.archived = false
	b.closed = false
	b.data = b.data[:0]
	return b
}

// If a new buffer was created, the new head is returnd.
// Otherwise, the existing buffer is returnd.
// Normaly, only "newer" data should be written, but if the value would
// end up in the same buffer anyways it is allowed.
func (b *buffer) write(ts int64, value Float) (*buffer, error) {
	if ts < b.start {
		return nil, errors.New("cannot write value to buffer from past")
	}

	// idx := int((ts - b.start + (b.frequency / 3)) / b.frequency)
	idx := int((ts - b.start) / b.frequency)
	if idx >= cap(b.data) {
		newbuf := newBuffer(ts, b.frequency)
		newbuf.prev = b
		b.next = newbuf
		b.close()
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

func (b *buffer) end() int64 {
	return b.firstWrite() + int64(len(b.data))*b.frequency
}

func (b *buffer) firstWrite() int64 {
	return b.start + (b.frequency / 2)
}

func (b *buffer) close() {}

/*
func (b *buffer) close() {
	if b.closed {
		return
	}

	b.closed = true
	n, sum, min, max := 0, 0., math.MaxFloat64, -math.MaxFloat64
	for _, x := range b.data {
		if x.IsNaN() {
			continue
		}

		n += 1
		f := float64(x)
		sum += f
		min = math.Min(min, f)
		max = math.Max(max, f)
	}

	b.statisticts.samples = n
	if n > 0 {
		b.statisticts.avg = Float(sum / float64(n))
		b.statisticts.min = Float(min)
		b.statisticts.max = Float(max)
	} else {
		b.statisticts.avg = NaN
		b.statisticts.min = NaN
		b.statisticts.max = NaN
	}
}
*/

// func interpolate(idx int, data []Float) Float {
// 	if idx == 0 || idx+1 == len(data) {
// 		return NaN
// 	}
// 	return (data[idx-1] + data[idx+1]) / 2.0
// }

// Return all known values from `from` to `to`. Gaps of information are represented as NaN.
// Simple linear interpolation is done between the two neighboring cells if possible.
// If values at the start or end are missing, instead of NaN values, the second and thrid
// return values contain the actual `from`/`to`.
// This function goes back the buffer chain if `from` is older than the currents buffer start.
// The loaded values are added to `data` and `data` is returned, possibly with a shorter length.
// If `data` is not long enough to hold all values, this function will panic!
func (b *buffer) read(from, to int64, data []Float) ([]Float, int64, int64, error) {
	if from < b.firstWrite() {
		if b.prev != nil {
			return b.prev.read(from, to, data)
		}
		from = b.firstWrite()
	}

	var i int = 0
	var t int64 = from
	for ; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			if b.next == nil {
				break
			}
			b = b.next
			idx = 0
		}

		if idx >= len(b.data) {
			if b.next == nil || to <= b.next.start {
				break
			}
			data[i] += NaN
		} else if t < b.start {
			data[i] += NaN
			// } else if b.data[idx].IsNaN() {
			// 	data[i] += interpolate(idx, b.data)
		} else {
			data[i] += b.data[idx]
		}
		i++
	}

	return data[:i], from, t, nil
}

// Returns true if this buffer needs to be freed.
func (b *buffer) free(t int64) (delme bool, n int) {
	if b.prev != nil {
		delme, m := b.prev.free(t)
		n += m
		if delme {
			b.prev.next = nil
			if cap(b.prev.data) == BUFFER_CAP {
				bufferPool.Put(b.prev)
			}
			b.prev = nil
		}
	}

	end := b.end()
	if end < t {
		return true, n + 1
	}

	return false, n
}

// Call `callback` on every buffer that contains data in the range from `from` to `to`.
func (b *buffer) iterFromTo(from, to int64, callback func(b *buffer) error) error {
	if b == nil {
		return nil
	}

	if err := b.prev.iterFromTo(from, to, callback); err != nil {
		return err
	}

	if from <= b.end() && b.start <= to {
		return callback(b)
	}

	return nil
}

func (b *buffer) count() int64 {
	res := int64(len(b.data))
	if b.prev != nil {
		res += b.prev.count()
	}
	return res
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
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu0" }.
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
	}

	child = &level{
		metrics:  make([]*buffer, nMetrics),
		children: nil,
	}

	if l.children != nil {
		l.children[selector[0]] = child
	} else {
		l.children = map[string]*level{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:], nMetrics)
}

func (l *level) free(t int64) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	n := 0
	for i, b := range l.metrics {
		if b != nil {
			delme, m := b.free(t)
			n += m
			if delme {
				if cap(b.data) == BUFFER_CAP {
					bufferPool.Put(b)
				}
				l.metrics[i] = nil
			}
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

func (l *level) sizeInBytes() int64 {
	l.lock.RLock()
	defer l.lock.RUnlock()
	size := int64(0)

	for _, b := range l.metrics {
		if b != nil {
			size += b.count() * int64(unsafe.Sizeof(Float(0)))
		}
	}

	for _, child := range l.children {
		size += child.sizeInBytes()
	}

	return size
}

type MemoryStore struct {
	root    level // root of the tree structure
	metrics map[string]MetricConfig
}

// Return a new, initialized instance of a MemoryStore.
// Will panic if values in the metric configurations are invalid.
func NewMemoryStore(metrics map[string]MetricConfig) *MemoryStore {
	offset := 0
	for key, config := range metrics {
		if config.Frequency == 0 {
			panic("invalid frequency")
		}

		metrics[key] = MetricConfig{
			Frequency:   config.Frequency,
			Aggregation: config.Aggregation,
			offset:      offset,
		}
		offset += 1
	}

	return &MemoryStore{
		root: level{
			metrics:  make([]*buffer, len(metrics)),
			children: make(map[string]*level),
		},
		metrics: metrics,
	}
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	var ok bool
	for i, metric := range metrics {
		if metric.mc.Frequency == 0 {
			metric.mc, ok = m.metrics[metric.Name]
			if !ok {
				metric.mc.Frequency = 0
			}
			metrics[i] = metric
		}
	}

	return m.WriteToLevel(&m.root, selector, ts, metrics)
}

func (m *MemoryStore) GetLevel(selector []string) *level {
	return m.root.findLevelOrCreate(selector, len(m.metrics))
}

// Assumes that `minfo` in `metrics` is filled in!
func (m *MemoryStore) WriteToLevel(l *level, selector []string, ts int64, metrics []Metric) error {
	l = l.findLevelOrCreate(selector, len(m.metrics))
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		if metric.mc.Frequency == 0 {
			continue
		}

		b := l.metrics[metric.mc.offset]
		if b == nil {
			// First write to this metric and level
			b = newBuffer(ts, metric.mc.Frequency)
			l.metrics[metric.mc.offset] = b
		}

		nb, err := b.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write created a new buffer...
		if b != nb {
			l.metrics[metric.mc.offset] = nb
		}
	}
	return nil
}

// Returns all values for metric `metric` from `from` to `to` for the selected level(s).
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// The second and third return value are the actual from/to for the data. Those can be different from
// the range asked for if no data was available.
func (m *MemoryStore) Read(selector Selector, metric string, from, to int64) ([]Float, int64, int64, error) {
	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, data := 0, make([]Float, (to-from)/minfo.Frequency+1)
	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer) error {
		cdata, cfrom, cto, err := b.read(from, to, data)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto || len(data) != len(cdata) {
			missingfront, missingback := int((from-cfrom)/minfo.Frequency), int((to-cto)/minfo.Frequency)
			if missingfront != 0 {
				return ErrDataDoesNotAlign
			}

			newlen := len(cdata) - missingback
			if newlen < 1 {
				return ErrDataDoesNotAlign
			}
			cdata = cdata[0:newlen]
			if len(cdata) != len(data) {
				return ErrDataDoesNotAlign
			}

			from, to = cfrom, cto
		}

		data = cdata
		n += 1
		return nil
	})

	if err != nil {
		return nil, 0, 0, err
	} else if n == 0 {
		return nil, 0, 0, errors.New("metric or host not found")
	} else if n > 1 {
		if minfo.Aggregation == AvgAggregation {
			normalize := 1. / Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if minfo.Aggregation != SumAggregation {
			return nil, 0, 0, errors.New("invalid aggregation")
		}
	}

	return data, from, to, nil
}

// Release all buffers for the selected level and all its children that contain only
// values older than `t`.
func (m *MemoryStore) Free(selector []string, t int64) (int, error) {
	return m.GetLevel(selector).free(t)
}

func (m *MemoryStore) FreeAll() error {
	for k := range m.root.children {
		delete(m.root.children, k)
	}

	return nil
}

func (m *MemoryStore) SizeInBytes() int64 {
	return m.root.sizeInBytes()
}

// Given a selector, return a list of all children of the level selected.
func (m *MemoryStore) ListChildren(selector []string) []string {
	lvl := &m.root
	for lvl != nil && len(selector) != 0 {
		lvl.lock.RLock()
		next := lvl.children[selector[0]]
		lvl.lock.RUnlock()
		lvl = next
		selector = selector[1:]
	}

	if lvl == nil {
		return nil
	}

	lvl.lock.RLock()
	defer lvl.lock.RUnlock()

	children := make([]string, 0, len(lvl.children))
	for child := range lvl.children {
		children = append(children, child)
	}

	return children
}
