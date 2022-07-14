package memstore

import (
	"fmt"
	"io"
	"reflect"
	"unsafe"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

// Can be done in parallel with other operations, but is single threaded itself.
func (ms *MemoryStore) SaveCheckpoint(from, to int64, w io.Writer) error {
	// Header:
	buf := make([]byte, 0, writeBufferSize*2)
	buf = append(buf, magicValue...)
	buf = encodeBytes(buf, nil)
	buf = encodeUint64(buf, uint64(from))
	buf = encodeUint64(buf, uint64(to))

	metricsbuf := make([]types.Float, 0, (to-from)/ms.MinFrequency()+1)
	var err error
	if buf, err = ms.root.saveCheckpoint(ms, from, to, w, buf, metricsbuf); err != nil {
		return err
	}

	if _, err := w.Write(buf); err != nil {
		return err
	}

	return nil
}

func (l *Level) saveCheckpoint(ms *MemoryStore, from, to int64, w io.Writer, buf []byte, metricsbuf []types.Float) ([]byte, error) {
	var err error
	l.lock.RLock()
	defer l.lock.RUnlock()

	buf = encodeBytes(buf, nil) // Reserved

	// Metrics:
	buf = encodeUint32(buf, uint32(len(l.metrics)))
	for i, c := range l.metrics {
		key := ms.GetMetricForOffset(i)
		buf = encodeString(buf, key)

		// Metric
		buf = encodeBytes(buf, nil) // Reserved

		metricsbuf = metricsbuf[:(to-from)/c.frequency+1]
		var cfrom int64
		if metricsbuf, cfrom, _, err = c.read(from, to, metricsbuf); err != nil {
			return nil, err
		}
		buf = encodeUint64(buf, uint64(c.frequency))
		buf = encodeUint64(buf, uint64(cfrom))
		buf = encodeUint32(buf, uint32(len(metricsbuf)))

		var x types.Float
		elmsize := unsafe.Sizeof(x)
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&metricsbuf))
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(sh.Data)), sh.Len*int(elmsize))
		buf = append(buf, bytes...)

		if len(buf) >= writeBufferSize {
			if _, err = w.Write(buf); err != nil {
				return nil, err
			}

			buf = buf[0:]
		}
	}

	// Sublevels:
	buf = encodeUint32(buf, uint32(len(l.sublevels)))
	for key, sublevel := range l.sublevels {
		buf = encodeString(buf, key)
		buf, err = sublevel.saveCheckpoint(ms, from, to, w, buf, metricsbuf)
		if err != nil {
			return nil, err
		}
	}

	return buf, nil
}

func (ms *MemoryStore) LoadCheckpoint(r io.Reader) error {
	buf := make([]byte, len(magicValue), 64)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	if string(buf) != magicValue {
		return fmt.Errorf("file corrupted: expected the file to start with %#v (got %#v)", magicValue, string(buf))
	}

	if _, err := decodeBytes(buf, r); err != nil { // Reserved
		return err
	}
	if _, err := decodeUint64(buf, r); err != nil { // From
		return err
	}
	if _, err := decodeUint64(buf, r); err != nil { // To
		return err
	}

	if err := ms.root.loadCheckpoint(ms, r, buf); err != nil {
		return err
	}

	return nil
}

// Blocks all other accesses for this level and all its sublevels!
func (l *Level) loadCheckpoint(ms *MemoryStore, r io.Reader, buf []byte) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	var n uint32
	var err error
	var key string
	if _, err = decodeBytes(buf, r); err != nil { // Reserved...
		return err
	}

	// Metrics:
	if n, err = decodeUint32(buf, r); err != nil {
		return err
	}
	for i := 0; i < int(n); i++ {
		if key, err = decodeString(buf, r); err != nil {
			return err
		}
		if l.metrics == nil {
			l.metrics = make([]*chunk, len(ms.metrics))
		}

		// Metric:
		if _, err = decodeBytes(buf, r); err != nil { // Reserved...
			return err
		}
		var freq, from uint64
		if freq, err = decodeUint64(buf, r); err != nil {
			return err
		}
		if from, err = decodeUint64(buf, r); err != nil {
			return err
		}
		numelements, err := decodeUint32(buf, r)
		if err != nil {
			return err
		}

		var x types.Float
		elmsize := unsafe.Sizeof(x)
		bytes := RequestBytes(int(elmsize) * int(numelements))
		if _, err = io.ReadFull(r, bytes); err != nil {
			return fmt.Errorf("loading metric %#v: %w", key, err)
		}

		metricConf, ok := ms.GetMetricConf(key)
		if !ok {
			// Skip unkown metrics
			ReleaseBytes(bytes)
			continue
		}

		sh := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
		chunk := &chunk{
			frequency:    int64(freq),
			start:        int64(from),
			prev:         nil,
			next:         nil,
			data:         unsafe.Slice((*types.Float)(unsafe.Pointer(sh.Data)), numelements),
			checkpointed: true,
		}

		if prevchunk := l.metrics[metricConf.Offset]; prevchunk != nil {
			if prevchunk.end() > chunk.start {
				return fmt.Errorf(
					"loading metric %#v: loaded checkpoint overlaps with other chunks or is not loaded in correct order (%d - %d)",
					key, prevchunk.start, chunk.start)
			}
			prevchunk.next = chunk
			chunk.prev = prevchunk
			l.metrics[metricConf.Offset] = chunk
		} else {
			l.metrics[metricConf.Offset] = chunk
		}
	}

	// Sublevels:
	if n, err = decodeUint32(buf, r); err != nil {
		return err
	}
	for i := 0; i < int(n); i++ {
		if key, err = decodeString(buf, r); err != nil {
			return err
		}
		if l.sublevels == nil {
			l.sublevels = make(map[string]*Level, n)
		}
		sublevel, ok := l.sublevels[key]
		if !ok {
			sublevel = &Level{}
		}

		if err = sublevel.loadCheckpoint(ms, r, buf); err != nil {
			return fmt.Errorf("loading sublevel %#v: %w", key, err)
		}

		if !ok {
			l.sublevels[key] = sublevel
		}
	}

	return nil
}
