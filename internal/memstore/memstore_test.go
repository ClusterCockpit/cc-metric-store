package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestMemoryStoreBasics(t *testing.T) {
	frequency := int64(10)
	start, count := int64(100), int64(5000)
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: frequency},
		"b": {Frequency: frequency * 2},
	})

	for i := int64(0); i < count; i++ {
		err := store.Write([]string{"testhost"}, start+i*frequency, []Metric{
			{Name: "a", Value: Float(i)},
			{Name: "b", Value: Float(i / 2)},
		})
		if err != nil {
			t.Error(err)
			return
		}
	}

	sel := Selector{{String: "testhost"}}
	adata, from, to, err := store.Read(sel, "a", start, start+count*frequency)
	if err != nil || from != start || to != start+count*frequency {
		t.Error(err)
		return
	}
	bdata, _, _, err := store.Read(sel, "b", start, start+count*frequency)
	if err != nil {
		t.Error(err)
		return
	}

	if len(adata) != int(count) || len(bdata) != int(count/2) {
		t.Error("unexpected count of returned values")
		return
	}

	for i := 0; i < int(count); i++ {
		if adata[i] != Float(i) {
			t.Errorf("incorrect value for metric a (%f vs. %f)", adata[i], Float(i))
			return
		}
	}

	for i := 0; i < int(count/2); i++ {
		if bdata[i] != Float(i) && bdata[i] != Float(i-1) {
			t.Errorf("incorrect value for metric b (%f) at index %d", bdata[i], i)
			return
		}
	}

}

func TestMemoryStoreTooMuchWrites(t *testing.T) {
	frequency := int64(10)
	count := BUFFER_CAP*3 + 10
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: frequency},
		"b": {Frequency: frequency * 2},
		"c": {Frequency: frequency / 2},
		"d": {Frequency: frequency * 3},
	})

	start := int64(100)
	for i := 0; i < count; i++ {
		if err := store.Write([]string{"test"}, start+int64(i)*frequency, []Metric{
			{Name: "a", Value: Float(i)},
			{Name: "b", Value: Float(i / 2)},
			{Name: "c", Value: Float(i * 2)},
			{Name: "d", Value: Float(i / 3)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	end := start + int64(count)*frequency
	data, from, to, err := store.Read(Selector{{String: "test"}}, "a", start, end)
	if len(data) != count || from != start || to != end || err != nil {
		t.Fatalf("a: err=%#v, from=%d, to=%d, data=%#v\n", err, from, to, data)
	}

	data, from, to, err = store.Read(Selector{{String: "test"}}, "b", start, end)
	if len(data) != count/2 || from != start || to != end || err != nil {
		t.Fatalf("b: err=%#v, from=%d, to=%d, data=%#v\n", err, from, to, data)
	}

	data, from, to, err = store.Read(Selector{{String: "test"}}, "c", start, end)
	if len(data) != count*2-1 || from != start || to != end-frequency/2 || err != nil {
		t.Fatalf("c: err=%#v, from=%d, to=%d, data=%#v\n", err, from, to, data)
	}

	data, from, to, err = store.Read(Selector{{String: "test"}}, "d", start, end)
	if len(data) != count/3+1 || from != start || to != end+frequency*2 || err != nil {
		t.Errorf("expected: err=nil, from=%d, to=%d, len(data)=%d\n", start, end+frequency*2, count/3)
		t.Fatalf("d: err=%#v, from=%d, to=%d, data=%#v\n", err, from, to, data)
	}
}

func TestMemoryStoreOutOfBounds(t *testing.T) {
	count := 2000
	toffset := 1000
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 60},
	})

	for i := 0; i < count; i++ {
		if err := store.Write([]string{"cluster", "host", "cpu"}, int64(toffset+i*60), []Metric{
			{Name: "a", Value: Float(i)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	sel := Selector{{String: "cluster"}, {String: "host"}, {String: "cpu"}}
	data, from, to, err := store.Read(sel, "a", 500, int64(toffset+count*60+500))
	if err != nil {
		t.Fatal(err)
	}

	if from/60 != int64(toffset)/60 || to/60 != int64(toffset+count*60)/60 {
		t.Fatalf("Got %d-%d, expected %d-%d", from, to, toffset, toffset+count*60)
	}

	if len(data) != count || data[0] != 0 || data[len(data)-1] != Float((count-1)) {
		t.Fatalf("Wrong data (got: %d, %f, %f, expected: %d, %f, %f)",
			len(data), data[0], data[len(data)-1], count, 0., Float(count-1))
	}

	testfrom, testlen := int64(100000000), int64(10000)
	data, from, to, err = store.Read(sel, "a", testfrom, testfrom+testlen)
	if len(data) != 0 || from != testfrom || to != testfrom || err != nil {
		t.Fatal("Unexpected data returned when reading range after valid data")
	}

	testfrom, testlen = 0, 10
	data, from, to, err = store.Read(sel, "a", testfrom, testfrom+testlen)
	if len(data) != 0 || from/60 != int64(toffset)/60 || to/60 != int64(toffset)/60 || err != nil {
		t.Fatal("Unexpected data returned when reading range before valid data")
	}
}

func TestMemoryStoreMissingDatapoints(t *testing.T) {
	count := 3000
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
	})

	for i := 0; i < count; i++ {
		if i%3 != 0 {
			continue
		}

		err := store.Write([]string{"testhost"}, int64(i), []Metric{
			{Name: "a", Value: Float(i)},
		})
		if err != nil {
			t.Error(err)
			return
		}
	}

	sel := Selector{{String: "testhost"}}
	adata, _, _, err := store.Read(sel, "a", 0, int64(count))
	if err != nil {
		t.Error(err)
		return
	}

	if len(adata) != count-2 {
		t.Error("unexpected len")
		return
	}

	for i := 0; i < count-2; i++ {
		if i%3 == 0 {
			if adata[i] != Float(i) {
				t.Error("unexpected value")
				return
			}
		} else {
			if !math.IsNaN(float64(adata[i])) {
				t.Errorf("NaN expected (i = %d, value = %f)\n", i, adata[i])
				return
			}
		}
	}
}

func TestMemoryStoreAggregation(t *testing.T) {
	count := 3000
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1, Aggregation: SumAggregation},
	})

	for i := 0; i < count; i++ {
		err := store.Write([]string{"host0", "cpu0"}, int64(i), []Metric{
			{Name: "a", Value: Float(i) / 2.},
		})
		if err != nil {
			t.Error(err)
			return
		}

		err = store.Write([]string{"host0", "cpu1"}, int64(i), []Metric{
			{Name: "a", Value: Float(i) * 2.},
		})
		if err != nil {
			t.Error(err)
			return
		}
	}

	adata, from, to, err := store.Read(Selector{{String: "host0"}}, "a", int64(0), int64(count))
	if err != nil {
		t.Error(err)
		return
	}

	if len(adata) != count || from != 0 || to != int64(count) {
		t.Error("unexpected length or time range of returned data")
		return
	}

	for i := 0; i < count; i++ {
		expected := Float(i)/2. + Float(i)*2.
		if adata[i] != expected {
			t.Errorf("expected: %f, got: %f", expected, adata[i])
			return
		}
	}
}

func TestMemoryStoreStats(t *testing.T) {
	count := 3000
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 1, Aggregation: AvgAggregation},
	})

	sel1 := []string{"cluster", "host1"}
	sel2 := []string{"cluster", "host2", "left"}
	sel3 := []string{"cluster", "host2", "right"}

	samples := 0
	asum, amin, amax := 0., math.MaxFloat32, -math.MaxFloat32
	bsum, bmin, bmax := 0., math.MaxFloat32, -math.MaxFloat32

	for i := 0; i < count; i++ {
		if i%5 == 0 {
			// Skip some writes, test if samples is calculated correctly
			continue
		}

		samples += 1
		a := float64(rand.Int()%100 - 50)
		asum += a
		amin = math.Min(amin, a)
		amax = math.Max(amax, a)
		b := float64(rand.Int()%100 - 50)
		bsum += b * 2
		bmin = math.Min(bmin, b)
		bmax = math.Max(bmax, b)

		store.Write(sel1, int64(i), []Metric{
			{Name: "a", Value: Float(a)},
		})
		store.Write(sel2, int64(i), []Metric{
			{Name: "b", Value: Float(b)},
		})
		store.Write(sel3, int64(i), []Metric{
			{Name: "b", Value: Float(b)},
		})
	}

	stats, from, to, err := store.Stats(Selector{{String: "cluster"}, {String: "host1"}}, "a", 0, int64(count))
	if err != nil {
		t.Fatal(err)
	}

	if from != 1 || to != int64(count) || stats.Samples != samples {
		t.Fatalf("unexpected: from=%d, to=%d, stats.Samples=%d (expected samples=%d)\n", from, to, stats.Samples, samples)
	}

	if stats.Avg != Float(asum/float64(samples)) || stats.Min != Float(amin) || stats.Max != Float(amax) {
		t.Fatalf("wrong stats: %#v\n", stats)
	}

	stats, from, to, err = store.Stats(Selector{{String: "cluster"}, {String: "host2"}}, "b", 0, int64(count))
	if err != nil {
		t.Fatal(err)
	}

	if from != 1 || to != int64(count) || stats.Samples != samples*2 {
		t.Fatalf("unexpected: from=%d, to=%d, stats.Samples=%d (expected samples=%d)\n", from, to, stats.Samples, samples*2)
	}

	if stats.Avg != Float(bsum/float64(samples*2)) || stats.Min != Float(bmin) || stats.Max != Float(bmax) {
		t.Fatalf("wrong stats: %#v (expected: avg=%f, min=%f, max=%f)\n", stats, bsum/float64(samples*2), bmin, bmax)
	}
}

func TestMemoryStoreArchive(t *testing.T) {
	store1 := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 1},
	})

	count := 2000
	for i := 0; i < count; i++ {
		err := store1.Write([]string{"cluster", "host", "cpu0"}, 100+int64(i), []Metric{
			{Name: "a", Value: Float(i)},
			{Name: "b", Value: Float(i * 2)},
		})
		if err != nil {
			t.Error(err)
			return
		}
	}

	// store1.DebugDump(bufio.NewWriter(os.Stdout))

	archiveRoot := t.TempDir()
	_, err := store1.ToCheckpoint(archiveRoot, 100, 100+int64(count/2))
	if err != nil {
		t.Error(err)
		return
	}

	_, err = store1.ToCheckpoint(archiveRoot, 100+int64(count/2), 100+int64(count))
	if err != nil {
		t.Error(err)
		return
	}

	store2 := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 1},
	})
	n, err := store2.FromCheckpoint(archiveRoot, 100)
	if err != nil {
		t.Error(err)
		return
	}

	sel := Selector{{String: "cluster"}, {String: "host"}, {String: "cpu0"}}
	adata, from, to, err := store2.Read(sel, "a", 100, int64(100+count))
	if err != nil {
		t.Error(err)
		return
	}

	if n != 2 || len(adata) != count || from != 100 || to != int64(100+count) {
		t.Errorf("unexpected: n=%d, len=%d, from=%d, to=%d\n", n, len(adata), from, to)
		return
	}

	for i := 0; i < count; i++ {
		expected := Float(i)
		if adata[i] != expected {
			t.Errorf("expected: %f, got: %f", expected, adata[i])
		}
	}
}

func TestMemoryStoreFree(t *testing.T) {
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 2},
	})

	count := 3000
	sel := []string{"cluster", "host", "1"}
	for i := 0; i < count; i++ {
		err := store.Write(sel, int64(i), []Metric{
			{Name: "a", Value: Float(i)},
			{Name: "b", Value: Float(i)},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := store.Free([]string{"cluster", "host"}, int64(BUFFER_CAP*2)+100)
	if err != nil {
		t.Fatal(err)
	}

	if n != 3 {
		t.Fatal("two buffers expected to be released")
	}

	adata, from, to, err := store.Read(Selector{{String: "cluster"}, {String: "host"}, {String: "1"}}, "a", 0, int64(count))
	if err != nil {
		t.Fatal(err)
	}

	if from != int64(BUFFER_CAP*2) || to != int64(count) || len(adata) != count-2*BUFFER_CAP {
		t.Fatalf("unexpected values from call to `Read`: from=%d, to=%d, len=%d", from, to, len(adata))
	}

	// bdata, from, to, err := store.Read(Selector{{String: "cluster"}, {String: "host"}, {String: "1"}}, "b", 0, int64(count))
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// if from != int64(BUFFER_CAP*2) || to != int64(count) || len(bdata) != (count-2*BUFFER_CAP)/2 {
	// 	t.Fatalf("unexpected values from call to `Read`: from=%d (expected: %d), to=%d (expected: %d), len=%d (expected: %d)",
	// 		from, BUFFER_CAP*2, to, count, len(bdata), (count-2*BUFFER_CAP)/2)
	// }

	if adata[0] != Float(BUFFER_CAP*2) || adata[len(adata)-1] != Float(count-1) {
		t.Fatal("wrong values")
	}
}

func BenchmarkMemoryStoreConcurrentWrites(b *testing.B) {
	frequency := int64(5)
	count := b.N
	goroutines := 4
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: frequency},
	})

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			host := fmt.Sprintf("host%d", g)
			for i := 0; i < count; i++ {
				store.Write([]string{"cluster", host, "cpu0"}, int64(i)*frequency, []Metric{
					{Name: "a", Value: Float(i)},
				})
			}
			wg.Done()
		}(g)
	}

	wg.Wait()
	b.StopTimer()

	for g := 0; g < goroutines; g++ {
		host := fmt.Sprintf("host%d", g)
		sel := Selector{{String: "cluster"}, {String: host}, {String: "cpu0"}}
		adata, _, _, err := store.Read(sel, "a", 0, int64(count)*frequency)
		if err != nil {
			b.Error(err)
			return
		}

		if len(adata) != count {
			b.Error("unexpected count")
			return
		}

		for i := 0; i < count; i++ {
			expected := Float(i)
			if adata[i] != expected {
				b.Error("incorrect value for metric a")
				return
			}
		}
	}
}

func BenchmarkMemoryStoreAggregation(b *testing.B) {
	b.StopTimer()
	count := 2000
	store := NewMemoryStore(map[string]MetricConfig{
		"flops_any": {Frequency: 1, Aggregation: AvgAggregation},
	})

	sel := []string{"testcluster", "host123", "cpu0"}
	for i := 0; i < count; i++ {
		sel[2] = "cpu0"
		err := store.Write(sel, int64(i), []Metric{
			{Name: "flops_any", Value: Float(i)},
		})
		if err != nil {
			b.Fatal(err)
		}

		sel[2] = "cpu1"
		err = store.Write(sel, int64(i), []Metric{
			{Name: "flops_any", Value: Float(i)},
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StartTimer()
	for n := 0; n < b.N; n++ {
		data, from, to, err := store.Read(Selector{{String: "testcluster"}, {String: "host123"}}, "flops_any", 0, int64(count))
		if err != nil {
			b.Fatal(err)
		}

		if len(data) != count || from != 0 || to != int64(count) {
			b.Fatal()
		}
	}
}
