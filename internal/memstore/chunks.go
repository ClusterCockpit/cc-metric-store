package memstore

import (
	"errors"
	"math"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

type chunk struct {
	frequency    int64         // Time between two "slots"
	start        int64         // Timestamp of when `data[0]` was written.
	prev, next   *chunk        // `prev` contains older data, `next` newer data.
	data         []types.Float // The slice should never reallocacte as `cap(data)` is respected.
	checkpointed bool          // If true, this buffer is already in a checkpoint on disk and full.
}

func newChunk(ts, freq int64) *chunk {
	return &chunk{
		frequency:    freq,
		start:        ts - (freq / 2),
		prev:         nil,
		next:         nil,
		checkpointed: false,
		data:         RequestFloatSlice(bufferSizeInFloats)[:0],
	}
}

func freeChunk(c *chunk) {
	ReleaseFloatSlice(c.data)
}

// If a new buffer was created, the new head is returnd.
// Otherwise, the existing buffer is returnd.
// Normaly, only "newer" data should be written, but if the value would
// end up in the same buffer anyways it is allowed.
func (c *chunk) write(ts int64, value types.Float) (*chunk, error) {
	if ts < c.start {
		return nil, errors.New("cannot write value to buffer from past")
	}

	// idx := int((ts - b.start + (b.frequency / 3)) / b.frequency)
	idx := int((ts - c.start) / c.frequency)
	if idx >= cap(c.data) {
		newchunk := newChunk(ts, c.frequency)
		newchunk.prev = c
		c.next = newchunk
		c = newchunk
		idx = 0
	}

	// Overwriting value or writing value from past
	if idx < len(c.data) {
		c.data[idx] = value
		return c, nil
	}

	// Fill up unwritten slots with NaN
	for i := len(c.data); i < idx; i++ {
		c.data = append(c.data, types.NaN)
	}

	c.data = append(c.data, value)
	return c, nil
}

func (c *chunk) end() int64 {
	return c.firstWrite() + int64(len(c.data))*c.frequency
}

func (c *chunk) firstWrite() int64 {
	return c.start + (c.frequency / 2)
}

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
func (c *chunk) read(from, to int64, data []types.Float) ([]types.Float, int64, int64, error) {
	if from < c.firstWrite() {
		if c.prev != nil {
			return c.prev.read(from, to, data)
		}
		from = c.firstWrite()
	}

	var i int = 0
	var t int64 = from
	for ; t < to; t += c.frequency {
		idx := int((t - c.start) / c.frequency)
		if idx >= cap(c.data) {
			if c.next == nil {
				break
			}
			c = c.next
			idx = 0
		}

		if idx >= len(c.data) {
			if c.next == nil || to <= c.next.start {
				break
			}
			data[i] += types.NaN
		} else if t < c.start {
			data[i] += types.NaN
			// } else if b.data[idx].IsNaN() {
			// 	data[i] += interpolate(idx, b.data)
		} else {
			data[i] += c.data[idx]
		}
		i++
	}

	return data[:i], from, t, nil
}

func (c *chunk) stats(from, to int64) (types.Stats, int64, int64, error) {
	stats := types.Stats{
		Samples: 0,
		Min:     types.Float(math.MaxFloat64),
		Avg:     0.0,
		Max:     types.Float(-math.MaxFloat64),
	}

	if from < c.firstWrite() {
		if c.prev != nil {
			return c.prev.stats(from, to)
		}
		from = c.firstWrite()
	}

	var i int = 0
	var t int64 = from
	for ; t < to; t += c.frequency {
		idx := int((t - c.start) / c.frequency)
		if idx >= cap(c.data) {
			if c.next == nil {
				break
			}
			c = c.next
			idx = 0
		}

		if idx >= len(c.data) {
			if c.next == nil || to <= c.next.start {
				break
			}
		} else if t >= c.start && !c.data[idx].IsNaN() {
			x := c.data[idx]
			stats.Samples += 1
			stats.Avg += x
			stats.Max = types.Float(math.Max(float64(x), float64(stats.Max)))
			stats.Min = types.Float(math.Min(float64(x), float64(stats.Min)))
		}
		i++
	}

	stats.Avg /= types.Float(stats.Samples)
	if stats.Samples == 0 {
		stats.Max = types.NaN
		stats.Min = types.NaN
	}
	return stats, from, t, nil
}

// Returns true if this buffer needs to be freed.
func (c *chunk) free(t int64) (delme bool, n int) {
	if c.prev != nil {
		delme, m := c.prev.free(t)
		n += m
		if delme {
			c.prev.next = nil
			freeChunk(c.prev)
			c.prev = nil
		}
	}

	end := c.end()
	if end < t {
		return true, n + 1
	}

	return false, n
}
