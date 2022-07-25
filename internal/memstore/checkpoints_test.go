package memstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"math"
	"reflect"
	"testing"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

func createTestStore(t *testing.T, withData bool) *MemoryStore {
	ms := NewMemoryStore(map[string]types.MetricConfig{
		"flops": {Frequency: 1},
		"membw": {Frequency: 1},
		"ipc":   {Frequency: 2},
	})

	if !withData {
		return ms
	}

	n := 1000
	sel := []string{"hello", "world"}
	for i := 0; i < n; i++ {
		if err := ms.Write(sel, int64(i), []types.Metric{
			{Name: "flops", Value: types.Float(math.Sin(float64(i) * 0.1))},
		}); err != nil {
			t.Fatal(err)
		}
	}

	// n := 3000
	// x1, x2, x3 := 0.0, 1.1, 2.2
	// d1, d2, d3 := 0.05, 0.1, 0.2

	// sel1, sel2, sel3 := []string{"cluster"}, []string{"cluster", "host1"}, []string{"cluster", "host2", "cpu0"}
	// for i := 0; i < n; i++ {
	// 	ms.Write(sel1, int64(i), []types.Metric{
	// 		{Name: "flops", Value: types.Float(x1)},
	// 		{Name: "membw", Value: types.Float(x2)},
	// 		{Name: "ipc", Value: types.Float(x3)},
	// 	})

	// 	ms.Write(sel2, int64(i), []types.Metric{
	// 		{Name: "flops", Value: types.Float(x1) + 1.},
	// 		{Name: "membw", Value: types.Float(x2) + 2.},
	// 		{Name: "ipc", Value: types.Float(x3) + 3.},
	// 	})

	// 	ms.Write(sel3, int64(i)*2, []types.Metric{
	// 		{Name: "flops", Value: types.Float(x1) + 1.},
	// 		{Name: "membw", Value: types.Float(x2) + 2.},
	// 		{Name: "ipc", Value: types.Float(x3) + 3.},
	// 	})

	// 	x1 += d1
	// 	x2 += d2
	// 	x3 += d3
	// }

	return ms
}

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

func TestStreamingCheckpointIndentity(t *testing.T) {
	disk := &bytes.Buffer{}
	ms1 := createTestStore(t, true)
	if err := ms1.SaveCheckpoint(0, 7000, disk); err != nil {
		t.Fatal("saving checkpoint failed: ", err)
	}

	// fmt.Printf("disk: %#v\n", disk.Bytes())

	ms2 := createTestStore(t, false)
	if err := ms2.LoadCheckpoint(disk); err != nil {
		t.Fatal("loading checkpoint failed: ", err)
	}

	arr1, from1, to1, err := ms1.Read(types.Selector{{String: "hello"}, {String: "world"}}, "flops", 0, 7000)
	if err != nil {
		t.Fatal(err)
	}

	arr2, from2, to2, err := ms2.Read(types.Selector{{String: "hello"}, {String: "world"}}, "flops", 0, 7000)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(arr1, arr2) || from1 != from2 || to1 != to2 {
		t.Fatal("x != deserialize(serialize(x))")
	}

	// if !reflect.DeepEqual(ms1, ms2) {
	// 	// fmt.Printf("ms1.root: %#v\n", ms1.root)
	// 	// fmt.Printf("ms2.root: %#v\n", ms2.root)
	// 	// fmt.Printf("ms1.root.sublevels['hello']: %#v\n", *ms1.root.sublevels["hello"])
	// 	// fmt.Printf("ms2.root.sublevels['hello']: %#v\n", *ms2.root.sublevels["hello"])
	// 	// fmt.Printf("ms1.root.sublevels['hello'].sublevels['world']: %#v\n", *ms1.root.sublevels["hello"].sublevels["world"])
	// 	// fmt.Printf("ms2.root.sublevels['hello'].sublevels['world']: %#v\n", *ms2.root.sublevels["hello"].sublevels["world"])
	// 	// fmt.Printf("ms1.root.sublevels['hello'].sublevels['world'].metrics[0]: %#v\n", *ms1.root.sublevels["hello"].sublevels["world"].metrics[0])
	// 	// fmt.Printf("ms2.root.sublevels['hello'].sublevels['world'].metrics[0]: %#v\n", *ms2.root.sublevels["hello"].sublevels["world"].metrics[0])

	// 	t.Fatal("x != deserialize(serialize(x))")
	// }
}
