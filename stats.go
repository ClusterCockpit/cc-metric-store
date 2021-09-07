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

// Return `Stats` by value for less allocations/GC?
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

// This function assmumes that `l.lock` is LOCKED!
// It basically works just like level.read but calculates min/max/avg for that data level.read would return.
// TODO: Make this DRY?
func (l *level) stats(metric string, from, to int64, aggregation string) (Stats, int64, int64, error) {
	if b, ok := l.metrics[metric]; ok {
		return b.stats(from, to)
	}

	if len(l.children) == 0 {
		return Stats{}, 0, 0, ErrNoData
	}

	n := 0
	samples := 0
	avg, min, max := Float(0), Float(math.MaxFloat32), Float(-math.MaxFloat32)
	for _, child := range l.children {
		child.lock.RLock()
		stats, cfrom, cto, err := child.stats(metric, from, to, aggregation)
		child.lock.RUnlock()

		if err == ErrNoData {
			continue
		}

		if err != nil {
			return Stats{}, 0, 0, err
		}

		if n == 0 {
			from = cfrom
			to = cto
		} else if cfrom != from || cto != to {
			return Stats{}, 0, 0, ErrDataDoesNotAlign
		}

		samples += stats.Samples
		avg += stats.Avg
		min = Float(math.Min(float64(min), float64(stats.Min)))
		max = Float(math.Max(float64(max), float64(stats.Max)))
		n += 1
	}

	if n == 0 {
		return Stats{}, 0, 0, ErrNoData
	}

	if aggregation == "avg" {
		avg /= Float(n)
	} else if aggregation != "sum" {
		return Stats{}, 0, 0, errors.New("invalid aggregation strategy: " + aggregation)
	}

	return Stats{
		Samples: samples,
		Avg:     avg,
		Min:     min,
		Max:     max,
	}, from, to, nil
}

func (m *MemoryStore) Stats(selector []string, metric string, from, to int64) (*Stats, int64, int64, error) {
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

	stats, from, to, err := l.stats(metric, from, to, minfo.Aggregation)
	return &stats, from, to, err
}
