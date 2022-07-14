package memstore

import (
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
