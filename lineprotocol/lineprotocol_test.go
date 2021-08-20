package lineprotocol

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	raw := `node,host=lousxps,cluster=test mem_used=4692.252,proc_total=1083,load_five=0.91,cpu_user=1.424336e+06,cpu_guest_nice=0,cpu_guest=0,mem_available=9829.848,mem_slab=514.796,mem_free=4537.956,proc_run=2,cpu_idle=2.1589764e+07,swap_total=0,mem_cached=6368.5,swap_free=0,load_fifteen=0.93,cpu_nice=196,cpu_softirq=41456,mem_buffers=489.992,mem_total=16088.7,load_one=0.84,cpu_system=517223,cpu_iowait=8994,cpu_steal=0,cpu_irq=113265,mem_sreclaimable=362.452 1629356936`
	expectedMeasurement := `node`
	expectedTags := map[string]string{
		"host":    "lousxps",
		"cluster": "test",
	}
	expectedFields := []Metric{
		{"mem_used", 4692.252},
		{"proc_total", 1083},
		{"load_five", 0.91},
		{"cpu_user", 1.424336e+06},
		{"cpu_guest_nice", 0},
		{"cpu_guest", 0},
		{"mem_available", 9829.848},
		{"mem_slab", 514.796},
		{"mem_free", 4537.956},
		{"proc_run", 2},
		{"cpu_idle", 2.1589764e+07},
		{"swap_total", 0},
		{"mem_cached", 6368.5},
		{"swap_free", 0},
		{"load_fifteen", 0.93},
		{"cpu_nice", 196},
		{"cpu_softirq", 41456},
		{"mem_buffers", 489.992},
		{"mem_total", 16088.7},
		{"load_one", 0.84},
		{"cpu_system", 517223},
		{"cpu_iowait", 8994},
		{"cpu_steal", 0},
		{"cpu_irq", 113265},
		{"mem_sreclaimable", 362.452},
	}
	expectedTimestamp := 1629356936

	line, err := Parse(raw)
	if err != nil {
		t.Error(err)
	}

	if line.measurement != expectedMeasurement {
		t.Error("measurement not as expected")
	}

	if line.t.Unix() != int64(expectedTimestamp) {
		t.Error("timestamp not as expected")
	}

	if !reflect.DeepEqual(line.tags, expectedTags) {
		t.Error("tags not as expected")
	}

	if !reflect.DeepEqual(line.fields, expectedFields) {
		t.Error("fields not as expected")
	}
}
