package memstore

import (
	"io"
	"reflect"
	"unsafe"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

func (l *level) streamingCheckpoint(ms *MemoryStore, from, to int64, w io.Writer, buf []byte, metricsbuf []types.Float) ([]byte, error) {
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
		buf, err = sublevel.streamingCheckpoint(ms, from, to, w, buf, metricsbuf)
		if err != nil {
			return nil, err
		}
	}

	return buf, nil
}
