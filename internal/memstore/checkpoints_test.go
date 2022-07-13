package memstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"reflect"
	"testing"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

func TestIntEncoding(t *testing.T) {
	buf := make([]byte, 0, 100)
	x1 := uint64(0x0102030405060708)
	buf = encodeUint64(buf, x1)

	x2, err := decodeUint64(make([]byte, 0, 10), bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		t.Fatal(err)
	}
	if x1 != x2 {
		t.Fatalf("uint64: x1 != x2: %x != %x", x1, x2)
	}

	buf = buf[0:0:100]
	x3 := uint32(0xabcde)
	buf = encodeUint32(buf, x3)

	x4, err := decodeUint32(make([]byte, 0, 10), bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		t.Fatal(err)
	}
	if x3 != x4 {
		t.Fatalf("uint32: x3 != x4: %x != %x", x3, x4)
	}
}

func TestIdentity(t *testing.T) {
	input := &Checkpoint{
		Reserved: []byte("Hallo Welt"),
		From:     1234,
		To:       5678,
		Root: &CheckpointLevel{
			Reserved: nil,
			Sublevels: map[string]*CheckpointLevel{
				"host1": {
					Reserved: []byte("some text..."),
					Metrics: map[string]*Metrics{
						"flops": {
							Reserved:  []byte("blablabla"),
							Frequency: 60,
							From:      12345,
							Data:      []types.Float{1.1, 2.2, 3.3, 4.4, 5.5, 6.6},
						},
						"xyz": {
							Frequency: 10,
							From:      123,
							Data:      []types.Float{1.2, 3.4, 5.6, 7.8},
						},
					},
				},
				"host2": {
					Sublevels: map[string]*CheckpointLevel{
						"cpu0": {
							Metrics: map[string]*Metrics{
								"abc": {
									Frequency: 42,
									From:      420,
									Data:      []types.Float{1., 2., 3., 4., 5., 6., 7., 8., 9., 10., 11., 12., 13., 14., 15.},
								},
							},
						},
					},
				},
			},
		},
	}

	disk := &bytes.Buffer{}
	if err := input.Serialize(disk); err != nil {
		t.Fatal("serialization failed:", err)
	}

	// fmt.Printf("disk.Len() = %d\n", disk.Len())
	// fmt.Printf("disk.Bytes() = %#v", disk.Bytes())

	output := &Checkpoint{}
	if err := output.Deserialize(bufio.NewReader(disk)); err != nil {
		t.Fatal("deserialization failed:", err)
	}

	if !reflect.DeepEqual(input, output) {
		input_json, err := json.Marshal(input)
		if err != nil {
			log.Fatal(err)
		}

		output_json, err := json.Marshal(output)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("a: %#v", string(input_json))
		log.Printf("b: %#v", string(output_json))
		t.Fatal("x != deserialize(serialize(x))")
	}
}
