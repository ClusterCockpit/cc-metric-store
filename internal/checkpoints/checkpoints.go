package checkpoints

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"unsafe"

	"github.com/ClusterCockpit/cc-metric-store/internal/memstore"
	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

const magicValue string = "CCMSv1.0" // MUST be 8 bytes!
const writeBufferSize int = 8192

// A single checkpoint file is defined by this structure.
type Checkpoint struct {
	Reserved []byte           // Reserved for future use/metadata.
	From, To uint64           // Time covered by this checkpoint.
	Root     *CheckpointLevel // Root of the internal tree structure.
}

// A single level in the tree of the in-memory store is defined by this structure.
type CheckpointLevel struct {
	Reserved  []byte                      // Reserved for future use/metadata.
	Metrics   map[string]*Metrics         // Payload...
	Sublevels map[string]*CheckpointLevel // Child in the internal tree structure.
}

// Payload/Data
type Metrics struct {
	Reserved  []byte        // Reserved for future use/metadata.
	Frequency uint64        // Frequency of measurements for this metric.
	From      uint64        // Timestamp of the first measurement of this metric.
	Data      []types.Float // Payload...
}

func (c *Checkpoint) Serialize(w io.Writer) error {
	// Write magic value (8 byte):
	if _, err := w.Write([]byte(magicValue)); err != nil {
		return err
	}

	// The header:
	buf := make([]byte, 0, writeBufferSize*2)
	buf = encodeBytes(buf, c.Reserved)
	buf = encodeUint64(buf, c.From)
	buf = encodeUint64(buf, c.To)

	// The rest:
	buf, err := c.Root.serialize(w, buf)
	if err != nil {
		return err
	}

	if _, err := w.Write(buf); err != nil {
		return err
	}

	return nil
}

func (c *CheckpointLevel) serialize(w io.Writer, buf []byte) ([]byte, error) {
	// The reserved data:
	buf = encodeBytes(buf, c.Reserved)

	var err error
	// metrics:
	buf = encodeUint32(buf, uint32(len(c.Metrics)))
	for key, metric := range c.Metrics {
		buf = encodeString(buf, key)
		buf, err = metric.serialize(w, buf)
		if err != nil {
			return nil, err
		}

		if len(buf) >= writeBufferSize {
			if _, err = w.Write(buf); err != nil {
				return nil, err
			}

			buf = buf[0:]
		}
	}

	// Sublevels:
	buf = encodeUint32(buf, uint32(len(c.Sublevels)))
	for key, sublevel := range c.Sublevels {
		buf = encodeString(buf, key)
		buf, err = sublevel.serialize(w, buf)
		if err != nil {
			return nil, err
		}
	}

	return buf, nil
}

func (m *Metrics) serialize(w io.Writer, buf []byte) ([]byte, error) {
	buf = encodeBytes(m.Reserved, buf)           // Reserved
	buf = encodeUint64(buf, m.Frequency)         // Frequency
	buf = encodeUint64(buf, m.From)              // First measurmenet timestamp
	buf = encodeUint32(buf, uint32(len(m.Data))) // Number of elements

	var x types.Float
	elmsize := unsafe.Sizeof(x)
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&m.Data))
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(sh.Data)), sh.Len*int(elmsize))

	buf = append(buf, bytes...)
	return buf, nil
}

func (c *Checkpoint) Deserialize(r *bufio.Reader) error {
	buf := make([]byte, 8, 16384)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}

	if string(buf) != magicValue {
		return fmt.Errorf("file corrupted: expected the file to start with %#v (got %#v)", magicValue, string(buf))
	}

	bytes, err := decodeBytes(buf, r)
	if err != nil {
		return err
	}
	c.Reserved = bytes

	if c.From, err = decodeUint64(buf, r); err != nil {
		return err
	}
	if c.To, err = decodeUint64(buf, r); err != nil {
		return err
	}

	c.Root = &CheckpointLevel{}
	return c.Root.deserialize(buf, r)
}

func (c *CheckpointLevel) deserialize(buf []byte, r *bufio.Reader) error {
	var err error
	if c.Reserved, err = decodeBytes(buf, r); err != nil {
		return err
	}

	// Metrics:
	n, err := decodeUint32(buf, r)
	if err != nil {
		return err
	}
	c.Metrics = make(map[string]*Metrics, n)
	for i := 0; i < int(n); i++ {
		key, err := decodeString(buf, r)
		if err != nil {
			return err
		}

		metrics := &Metrics{}
		if err := metrics.deserialize(buf, r); err != nil {
			return err
		}

		c.Metrics[key] = metrics
	}

	// Sublevels:
	n, err = decodeUint32(buf, r)
	if err != nil {
		return err
	}
	c.Sublevels = make(map[string]*CheckpointLevel, n)
	for i := 0; i < int(n); i++ {
		key, err := decodeString(buf, r)
		if err != nil {
			return err
		}

		sublevel := &CheckpointLevel{}
		if err := sublevel.deserialize(buf, r); err != nil {
			return err
		}

		c.Sublevels[key] = sublevel
	}

	return nil
}

func (m *Metrics) deserialize(buf []byte, r *bufio.Reader) error {
	bytes, err := decodeBytes(buf, r)
	if err != nil {
		return err
	}
	m.Reserved = bytes

	if m.Frequency, err = decodeUint64(buf, r); err != nil {
		return err
	}
	if m.From, err = decodeUint64(buf, r); err != nil {
		return err
	}

	n, err := decodeUint32(buf, r)
	if err != nil {
		return err
	}

	var x types.Float
	elmsize := unsafe.Sizeof(x)
	bytes = memstore.RequestBytes(int(n) * int(elmsize))
	if _, err := io.ReadFull(r, bytes); err != nil {
		return err
	}

	sh := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
	m.Data = unsafe.Slice((*types.Float)(unsafe.Pointer(sh.Data)), n)
	return nil
}

func encodeBytes(buf []byte, bytes []byte) []byte {
	buf = encodeUint32(buf, uint32(len(bytes)))
	buf = append(buf, bytes...)
	return buf
}

func encodeString(buf []byte, str string) []byte {
	buf = encodeUint32(buf, uint32(len(str)))
	buf = append(buf, str...)
	return buf
}

func encodeUint64(buf []byte, i uint64) []byte {
	buf = append(buf, 0, 0, 0, 0, 0, 0, 0, 0) // 8 bytes
	binary.LittleEndian.PutUint64(buf, i)
	return buf[8:]
}

func encodeUint32(buf []byte, i uint32) []byte {
	buf = append(buf, 0, 0, 0, 0) // 4 bytes
	binary.LittleEndian.PutUint32(buf, i)
	return buf[4:]
}

func decodeBytes(buf []byte, r *bufio.Reader) ([]byte, error) {
	len, err := decodeUint32(buf, r)
	if err != nil {
		return nil, err
	}

	bytes := make([]byte, len)
	if _, err := io.ReadFull(r, bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

func decodeString(buf []byte, r *bufio.Reader) (string, error) {
	len, err := decodeUint32(buf, r)
	if err != nil {
		return "", err
	}

	var bytes []byte
	if cap(buf) <= int(len) {
		bytes = buf[0:int(len)]
	} else {
		bytes = make([]byte, int(len))
	}

	if _, err := io.ReadFull(r, bytes); err != nil {
		return "", err
	}
	return string(bytes), nil
}

func decodeUint64(buf []byte, r *bufio.Reader) (uint64, error) {
	buf = buf[0:8]
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(buf), nil
}

func decodeUint32(buf []byte, r *bufio.Reader) (uint32, error) {
	buf = buf[0:4]
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(buf), nil
}
