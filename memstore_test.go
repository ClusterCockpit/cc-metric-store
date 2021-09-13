package main

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestMemoryStoreBasics(t *testing.T) {
	frequency := int64(3)
	count := int64(5000)
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: frequency},
		"b": {Frequency: frequency * 2},
	})

	for i := int64(0); i < count; i++ {
		err := store.Write([]string{"testhost"}, i*frequency, []Metric{
			{Name: "a", Value: Float(i)},
			{Name: "b", Value: Float(i) * 0.5},
		})
		if err != nil {
			t.Error(err)
			return
		}
	}

	sel := Selector{{String: "testhost"}}
	adata, from, to, err := store.Read(sel, "a", 0, count*frequency)
	if err != nil || from != 0 || to != count*frequency {
		t.Error(err)
		return
	}
	bdata, _, _, err := store.Read(sel, "b", 0, count*frequency)
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
		expected := Float(i) + 0.5
		if bdata[i] != expected {
			t.Errorf("incorrect value for metric b (%f vs. %f)", bdata[i], expected)
			return
		}
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

	if len(adata) != count {
		t.Error("unexpected len")
		return
	}

	for i := 0; i < count; i++ {
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
		"a": {Frequency: 1, Aggregation: "sum"},
		"b": {Frequency: 2, Aggregation: "avg"},
	})

	for i := 0; i < count; i++ {
		err := store.Write([]string{"host0", "cpu0"}, int64(i), []Metric{
			{Name: "a", Value: Float(i) / 2.},
			{Name: "b", Value: Float(i) * 2.},
		})
		if err != nil {
			t.Error(err)
			return
		}

		err = store.Write([]string{"host0", "cpu1"}, int64(i), []Metric{
			{Name: "a", Value: Float(i) * 2.},
			{Name: "b", Value: Float(i) / 2.},
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

	bdata, from, to, err := store.Read(Selector{{String: "host0"}}, "b", int64(0), int64(count))
	if err != nil {
		t.Error(err)
		return
	}

	if len(bdata) != count/2 || from != 0 || to != int64(count) {
		t.Error("unexpected length or time range of returned data")
		return
	}

	for i := 0; i < count/2; i++ {
		j := (i * 2) + 1
		expected := (Float(j)*2. + Float(j)*0.5) / 2.
		if bdata[i] != expected {
			t.Errorf("expected: %f, got: %f", expected, bdata[i])
			return
		}
	}
}

func TestMemoryStoreStats(t *testing.T) {
	count := 3000
	store := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 1, Aggregation: "avg"},
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

	n, err := store.Free(Selector{{String: "cluster"}, {String: "host"}}, int64(BUFFER_CAP*2)+100)
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

	bdata, from, to, err := store.Read(Selector{{String: "cluster"}, {String: "host"}, {String: "1"}}, "b", 0, int64(count))
	if err != nil {
		t.Fatal(err)
	}

	if from != int64(BUFFER_CAP*2) || to != int64(count) || len(bdata) != (count-2*BUFFER_CAP)/2 {
		t.Fatalf("unexpected values from call to `Read`: from=%d, to=%d, len=%d", from, to, len(bdata))
	}

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
		"flops_any": {Frequency: 1, Aggregation: "avg"},
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
