package memstore

import (
	"errors"
	"sync"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

// Could also be called "node" as this forms a node in a tree structure.
// Called level because "node" might be confusing here.
// Can be both a leaf or a inner node. In this tree structue, inner nodes can
// also hold data (in `metrics`).
type Level struct {
	lock      sync.RWMutex
	metrics   []*chunk          // Every level can store metrics.
	sublevels map[string]*Level // Lower levels.
}

// Find the correct level for the given selector, creating it if
// it does not exist. Example selector in the context of the
// ClusterCockpit could be: []string{ "emmy", "host123", "cpu0" }.
// This function would probably benefit a lot from `level.children` beeing a `sync.Map`?
func (l *Level) findLevelOrCreate(selector []string, nMetrics int) *Level {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	var child *Level
	var ok bool
	if l.sublevels == nil {
		// sublevels map needs to be created...
		l.lock.RUnlock()
	} else {
		child, ok := l.sublevels[selector[0]]
		l.lock.RUnlock()
		if ok {
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	}

	// The level does not exist, take write lock for unqiue access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	if l.sublevels != nil {
		child, ok = l.sublevels[selector[0]]
		if ok {
			l.lock.Unlock()
			return child.findLevelOrCreate(selector[1:], nMetrics)
		}
	}

	child = &Level{
		metrics:   make([]*chunk, nMetrics),
		sublevels: nil,
	}

	if l.sublevels != nil {
		l.sublevels[selector[0]] = child
	} else {
		l.sublevels = map[string]*Level{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:], nMetrics)
}

func (l *Level) findLevel(selector []string) *Level {
	if len(selector) == 0 {
		return l
	}

	l.lock.RLock()
	defer l.lock.RUnlock()

	lvl := l.sublevels[selector[0]]
	if lvl == nil {
		return nil
	}

	return lvl.findLevel(selector[1:])
}

func (l *Level) free(t int64) (delme bool, n int) {
	l.lock.Lock()
	defer l.lock.Unlock()

	for i, c := range l.metrics {
		if c != nil {
			delchunk, m := c.free(t)
			n += m
			if delchunk {
				freeChunk(c)
				l.metrics[i] = nil
			}
		}
	}

	for key, l := range l.sublevels {
		delsublevel, m := l.free(t)
		n += m
		if delsublevel {
			l.sublevels[key] = nil
		}
	}

	return len(l.metrics) == 0 && len(l.sublevels) == 0, n
}

type MemoryStore struct {
	root Level // root of the tree structure
	// TODO...

	metrics map[string]types.MetricConfig // TODO...
}

// Return a new, initialized instance of a MemoryStore.
// Will panic if values in the metric configurations are invalid.
func NewMemoryStore(metrics map[string]types.MetricConfig) *MemoryStore {
	offset := 0
	for key, config := range metrics {
		if config.Frequency == 0 {
			panic("invalid frequency")
		}

		metrics[key] = types.MetricConfig{
			Frequency:   config.Frequency,
			Aggregation: config.Aggregation,
			Offset:      offset,
		}
		offset += 1
	}

	return &MemoryStore{
		root: Level{
			metrics:   make([]*chunk, len(metrics)),
			sublevels: make(map[string]*Level),
		},
		metrics: metrics,
	}
}

func (ms *MemoryStore) GetMetricConf(metric string) (types.MetricConfig, bool) {
	conf, ok := ms.metrics[metric]
	return conf, ok
}

func (ms *MemoryStore) GetMetricForOffset(offset int) string {
	return "" // TODO!
}

func (ms *MemoryStore) MinFrequency() int64 {
	return 10 // TODO
}

func (m *MemoryStore) GetLevel(selector []string) *Level {
	return m.root.findLevelOrCreate(selector, len(m.metrics))
}

func (m *MemoryStore) WriteToLevel(l *Level, selector []string, ts int64, metrics []types.Metric) error {
	l = l.findLevelOrCreate(selector, len(m.metrics))
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		if metric.Conf.Frequency == 0 {
			continue
		}

		c := l.metrics[metric.Conf.Offset]
		if c == nil {
			// First write to this metric and level
			c = newChunk(ts, metric.Conf.Frequency)
			l.metrics[metric.Conf.Offset] = c
		}

		nc, err := c.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write started a new chunk...
		if c != nc {
			l.metrics[metric.Conf.Offset] = nc
		}
	}
	return nil
}

func (m *MemoryStore) Free(t int64) int {
	_, n := m.root.free(t)
	return n
}

func (l *Level) findBuffers(selector types.Selector, offset int, f func(c *chunk) error) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	if len(selector) == 0 {
		b := l.metrics[offset]
		if b != nil {
			return f(b)
		}

		for _, lvl := range l.sublevels {
			err := lvl.findBuffers(nil, offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	sel := selector[0]
	if len(sel.String) != 0 && l.sublevels != nil {
		lvl, ok := l.sublevels[sel.String]
		if ok {
			err := lvl.findBuffers(selector[1:], offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if sel.Group != nil && l.sublevels != nil {
		for _, key := range sel.Group {
			lvl, ok := l.sublevels[key]
			if ok {
				err := lvl.findBuffers(selector[1:], offset, f)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	if sel.Any && l.sublevels != nil {
		for _, lvl := range l.sublevels {
			if err := lvl.findBuffers(selector[1:], offset, f); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

var (
	ErrNoData           error = errors.New("no data for this metric/level")
	ErrDataDoesNotAlign error = errors.New("data from lower granularities does not align")
)

// Returns all values for metric `metric` from `from` to `to` for the selected level(s).
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// The second and third return value are the actual from/to for the data. Those can be different from
// the range asked for if no data was available.
func (m *MemoryStore) Read(selector types.Selector, metric string, from, to int64) ([]types.Float, int64, int64, error) {
	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	mc, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, data := 0, make([]types.Float, (to-from)/mc.Frequency+1)
	err := m.root.findBuffers(selector, mc.Offset, func(c *chunk) error {
		cdata, cfrom, cto, err := c.read(from, to, data)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto || len(data) != len(cdata) {
			missingfront, missingback := int((from-cfrom)/mc.Frequency), int((to-cto)/mc.Frequency)
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
		if mc.Aggregation == types.AvgAggregation {
			normalize := 1. / types.Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if mc.Aggregation != types.SumAggregation {
			return nil, 0, 0, errors.New("invalid aggregation")
		}
	}

	return data, from, to, nil
}
