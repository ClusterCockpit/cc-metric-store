package memstore

import (
	"bufio"
	"fmt"
	"io"
	"reflect"
	"unsafe"

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
	// The header:
	buf := make([]byte, 0, writeBufferSize*2)
	buf = append(buf, magicValue...)
	buf = encodeBytes(buf, c.Reserved)
	buf = encodeUint64(buf, c.From)
	buf = encodeUint64(buf, c.To)

	// The rest:
	var err error
	if buf, err = c.Root.serialize(w, buf); err != nil {
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
	buf = encodeBytes(buf, m.Reserved)           // Reserved
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
	buf := make([]byte, len(magicValue), 16384)
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
	if n != 0 {
		c.Metrics = make(map[string]*Metrics, n)
		for i := 0; i < int(n); i++ {
			key, err := decodeString(buf, r)
			if err != nil {
				return fmt.Errorf("decoding metric key: %w", err)
			}

			// log.Printf("decoding metric %#v...\n", key)
			metrics := &Metrics{}
			if err := metrics.deserialize(buf, r); err != nil {
				return fmt.Errorf("decoding metric for key %#v: %w", key, err)
			}

			c.Metrics[key] = metrics
		}
	}

	// Sublevels:
	n, err = decodeUint32(buf, r)
	if err != nil {
		return err
	}
	if n != 0 {
		c.Sublevels = make(map[string]*CheckpointLevel, n)
		for i := 0; i < int(n); i++ {
			key, err := decodeString(buf, r)
			if err != nil {
				return fmt.Errorf("decoding sublevel key: %w", err)
			}

			// log.Printf("decoding sublevel %#v...\n", key)
			sublevel := &CheckpointLevel{}
			if err := sublevel.deserialize(buf, r); err != nil {
				return fmt.Errorf("decoding sublevel for key %#v: %w", key, err)
			}

			c.Sublevels[key] = sublevel
		}
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
	bytes = RequestBytes(int(n) * int(elmsize))
	if _, err := io.ReadFull(r, bytes); err != nil {
		return fmt.Errorf("reading payload (n=%d, elmsize=%d): %w", n, elmsize, err)
	}

	sh := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
	m.Data = unsafe.Slice((*types.Float)(unsafe.Pointer(sh.Data)), n)
	return nil
}

func encodeBytes(buf []byte, bytes []byte) []byte {
	buf = encodeUint32(buf, uint32(len(bytes)))
	return append(buf, bytes...)
}

func encodeString(buf []byte, str string) []byte {
	buf = encodeUint32(buf, uint32(len(str)))
	return append(buf, str...)
}

func encodeUint64(buf []byte, i uint64) []byte {
	return append(buf,
		byte((i>>0)&0xff),
		byte((i>>8)&0xff),
		byte((i>>16)&0xff),
		byte((i>>24)&0xff),
		byte((i>>32)&0xff),
		byte((i>>40)&0xff),
		byte((i>>48)&0xff),
		byte((i>>56)&0xff))
}

func encodeUint32(buf []byte, i uint32) []byte {
	return append(buf,
		byte((i>>0)&0xff),
		byte((i>>8)&0xff),
		byte((i>>16)&0xff),
		byte((i>>24)&0xff))
}

func decodeBytes(buf []byte, r *bufio.Reader) ([]byte, error) {
	len, err := decodeUint32(buf, r)
	if err != nil {
		return nil, err
	}

	if len == 0 {
		return nil, nil
	}

	bytes := make([]byte, len)
	if _, err := io.ReadFull(r, bytes); err != nil {
		return nil, fmt.Errorf("decoding %d bytes: %w", len, err)
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

	return (uint64(buf[0]) << 0) |
		(uint64(buf[1]) << 8) |
		(uint64(buf[2]) << 16) |
		(uint64(buf[3]) << 24) |
		(uint64(buf[4]) << 32) |
		(uint64(buf[5]) << 40) |
		(uint64(buf[6]) << 48) |
		(uint64(buf[7]) << 56), nil
}

func decodeUint32(buf []byte, r *bufio.Reader) (uint32, error) {
	buf = buf[0:4]
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	return (uint32(buf[0]) << 0) |
		(uint32(buf[1]) << 8) |
		(uint32(buf[2]) << 16) |
		(uint32(buf[3]) << 24), nil
}
