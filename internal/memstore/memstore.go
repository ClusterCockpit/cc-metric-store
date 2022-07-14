package memstore

import "sync"

// Could also be called "node" as this forms a node in a tree structure.
// Called level because "node" might be confusing here.
// Can be both a leaf or a inner node. In this tree structue, inner nodes can
// also hold data (in `metrics`).
type level struct {
	lock      sync.RWMutex
	metrics   []*chunk          // Every level can store metrics.
	sublevels map[string]*level // Lower levels.
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

	child = &level{
		metrics:   make([]*chunk, nMetrics),
		sublevels: nil,
	}

	if l.sublevels != nil {
		l.sublevels[selector[0]] = child
	} else {
		l.sublevels = map[string]*level{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findLevelOrCreate(selector[1:], nMetrics)
}

func (l *level) free(t int64) (delme bool, n int) {
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
	root level // root of the tree structure
	// TODO...

	metrics map[string]int // TODO...
}

func (ms *MemoryStore) GetOffset(metric string) int {
	return -1 // TODO!
}

func (ms *MemoryStore) GetMetricForOffset(offset int) string {
	return "" // TODO!
}

func (ms *MemoryStore) MinFrequency() int64 {
	return 10 // TODO
}
