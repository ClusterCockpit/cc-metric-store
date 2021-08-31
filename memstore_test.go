package main

import (
	"fmt"
	"math"
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

	adata, from, to, err := store.Read([]string{"testhost"}, "a", 0, count*frequency)
	if err != nil || from != 0 || to != count*frequency {
		t.Error(err)
		return
	}
	bdata, _, _, err := store.Read([]string{"testhost"}, "b", 0, count*frequency)
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

	adata, _, _, err := store.Read([]string{"testhost"}, "a", 0, int64(count))
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

	adata, from, to, err := store.Read([]string{"host0"}, "a", int64(0), int64(count))
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

	bdata, from, to, err := store.Read([]string{"host0"}, "b", int64(0), int64(count))
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
	_, err := store1.ToArchive(archiveRoot, 100, 100+int64(count/2))
	if err != nil {
		t.Error(err)
		return
	}

	_, err = store1.ToArchive(archiveRoot, 100+int64(count/2), 100+int64(count))
	if err != nil {
		t.Error(err)
		return
	}

	store2 := NewMemoryStore(map[string]MetricConfig{
		"a": {Frequency: 1},
		"b": {Frequency: 1},
	})
	n, err := store2.FromArchive(archiveRoot, 100)
	if err != nil {
		t.Error(err)
		return
	}

	adata, from, to, err := store2.Read([]string{"cluster", "host", "cpu0"}, "a", 100, int64(100+count))
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
		adata, _, _, err := store.Read([]string{"cluster", host, "cpu0"}, "a", 0, int64(count)*frequency)
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
		data, from, to, err := store.Read(sel[0:2], "flops_any", 0, int64(count))
		if err != nil {
			b.Fatal(err)
		}

		if len(data) != count || from != 0 || to != int64(count) {
			b.Fatal()
		}
	}
}
