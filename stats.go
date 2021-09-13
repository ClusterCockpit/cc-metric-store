package main

import (
	"errors"
	"math"
)

type Stats struct {
	Samples int
	Avg     Float
	Min     Float
	Max     Float
}

func (b *buffer) stats(from, to int64) (Stats, int64, int64, error) {
	if from < b.start {
		if b.prev != nil {
			return b.prev.stats(from, to)
		}
		from = b.start
	}

	samples := 0
	sum, min, max := 0.0, math.MaxFloat32, -math.MaxFloat32

	var t int64
	for t = from; t < to; t += b.frequency {
		idx := int((t - b.start) / b.frequency)
		if idx >= cap(b.data) {
			b = b.next
			if b == nil {
				break
			}
			idx = 0
		}

		if t < b.start || idx >= len(b.data) {
			continue
		}

		xf := float64(b.data[idx])
		if math.IsNaN(xf) {
			continue
		}

		samples += 1
		sum += xf
		min = math.Min(min, xf)
		max = math.Max(max, xf)
	}

	return Stats{
		Samples: samples,
		Avg:     Float(sum) / Float(samples),
		Min:     Float(min),
		Max:     Float(max),
	}, from, t, nil
}

// Returns statistics for the requested metric on the selected node/level.
// Data is aggregated to the selected level the same way as in `MemoryStore.Read`.
// If `Stats.Samples` is zero, the statistics should not be considered as valid.
func (m *MemoryStore) Stats(selector Selector, metric string, from, to int64) (*Stats, int64, int64, error) {
	if from > to {
		return nil, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.metrics[metric]
	if !ok {
		return nil, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, samples := 0, 0
	avg, min, max := Float(0), math.MaxFloat32, -math.MaxFloat32
	err := m.root.findBuffers(selector, minfo.offset, func(b *buffer) error {
		stats, cfrom, cto, err := b.stats(from, to)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto {
			return ErrDataDoesNotAlign
		}

		samples += stats.Samples
		avg += stats.Avg
		min = math.Min(min, float64(stats.Min))
		max = math.Max(max, float64(stats.Max))
		n += 1
		return nil
	})

	if err != nil {
		return nil, 0, 0, err
	}

	if n == 0 {
		return nil, 0, 0, ErrNoData
	}

	if minfo.aggregation == AvgAggregation {
		avg /= Float(n)
	} else if n > 1 && minfo.aggregation != SumAggregation {
		return nil, 0, 0, errors.New("invalid aggregation")
	}

	return &Stats{
		Samples: samples,
		Avg:     avg,
		Min:     Float(min),
		Max:     Float(max),
	}, from, to, nil
}

// Return the newest value of the metric at offset `offset`.
// In case the level does not hold the metric itself,
// sum up the values from all lower levels.
func (l *level) peek(offset int) (Float, int) {
	b := l.metrics[offset]
	if b != nil {
		x := b.data[len(b.data)-1]
		return x, 1
	}

	n, sum := 0, Float(0)
	for _, lvl := range l.children {
		lvl.lock.RLock()
		x, m := lvl.peek(offset)
		lvl.lock.RUnlock()
		n += m
		sum += x
	}

	return sum, n
}

// Return the newest value for every metric of every node for the given cluster.
// All values are always aggregated to a node.
func (m *MemoryStore) Peek(cluster string) (map[string]map[string]Float, error) {
	m.root.lock.RLock()
	clusterLevel, ok := m.root.children[cluster]
	m.root.lock.RUnlock()
	if !ok {
		return nil, errors.New("no such cluster: " + cluster)
	}

	clusterLevel.lock.RLock()
	defer clusterLevel.lock.RUnlock()

	nodes := make(map[string]map[string]Float)
	for node, l := range clusterLevel.children {
		l.lock.RLock()
		metrics := make(map[string]Float)
		for metric, minfo := range m.metrics {
			x, n := l.peek(minfo.offset)
			if n > 1 {
				if minfo.aggregation == NoAggregation {
					return nil, errors.New("cannot aggregate: " + metric)
				} else if minfo.aggregation == AvgAggregation {
					x /= Float(n)
				}
			}
			metrics[metric] = x
		}
		nodes[node] = metrics
		l.lock.RUnlock()
	}

	return nodes, nil
}
