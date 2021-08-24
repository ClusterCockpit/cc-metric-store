package main

import (
	"fmt"
	"log"
	"math"
	"testing"

	"github.com/ClusterCockpit/cc-metric-store/lineprotocol"
)

var testMetrics [][]lineprotocol.Metric = [][]lineprotocol.Metric{
	{{"flops", 100.5}, {"mem_bw", 2088.67}},
	{{"flops", 180.5}, {"mem_bw", 4078.32}, {"mem_capacity", 1020}},
	{{"flops", 980.5}, {"mem_bw", 9078.32}, {"mem_capacity", 5010}},
	{{"flops", 940.5}, {"mem_bw", 9278.32}, {"mem_capacity", 6010}},
	{{"flops", 930.5}, {"mem_bw", 9378.32}, {"mem_capacity", 7010}},
	{{"flops", 980.5}, {"mem_bw", 9478.32}, {"mem_capacity", 8010}},
	{{"flops", 980.5}, {"mem_bw", 9478.32}, {"mem_capacity", 8010}},
	{{"flops", 980.5}, {"mem_bw", 9478.32}, {"mem_capacity", 8010}},
	{{"flops", 970.5}, {"mem_bw", 9178.32}, {"mem_capacity", 2010}},
	{{"flops", 970.5}, {"mem_bw", 9178.32}, {"mem_capacity", 2010}}}

var testMetricsAlt [][]lineprotocol.Metric = [][]lineprotocol.Metric{
	{{"flops", 120.5}, {"mem_bw", 2080.67}},
	{{"flops", 130.5}, {"mem_bw", 4071.32}, {"mem_capacity", 1120}},
	{{"flops", 940.5}, {"mem_bw", 9072.32}, {"mem_capacity", 5210}},
	{{"flops", 950.5}, {"mem_bw", 9273.32}, {"mem_capacity", 6310}},
	{{"flops", 960.5}, {"mem_bw", 9374.32}, {"mem_capacity", 7410}},
	{{"flops", 970.5}, {"mem_bw", 9475.32}, {"mem_capacity", 8510}},
	{{"flops", 990.5}, {"mem_bw", 9476.32}, {"mem_capacity", 8610}},
	{{"flops", 910.5}, {"mem_bw", 9477.32}, {"mem_capacity", 8710}},
	{{"flops", 920.5}, {"mem_bw", 9178.32}, {"mem_capacity", 2810}},
	{{"flops", 930.5}, {"mem_bw", 9179.32}, {"mem_capacity", 2910}}}

func dumpStoreBuffer(s *storeBuffer) {
	log.Printf("Start TS %d\n", s.start)
	ctr := 0

	for _, val := range s.store {
		fmt.Printf("%f\t", val)
		ctr++

		if ctr == 10 {
			fmt.Printf("\n")
			ctr = 0
		}
	}
}

func printMemStore(m *MemoryStore) {
	log.Println("########################")
	log.Printf("Frequency %d, Metrics %d Slots %d\n",
		m.frequency, m.numMetrics, m.numSlots)
	log.Println("##Offsets")
	for key, val := range m.offsets {
		log.Printf("\t%s = %d\n", key, val)
	}
	log.Println("##Containers")
	for key, c := range m.containers {
		log.Printf("ID %s\n", key)
		log.Println("###current")
		dumpStoreBuffer(c.current)
		log.Println("###next")
		dumpStoreBuffer(c.next)
	}
	log.Println("########################")
}

//############################
//#### Whitebox tests ########
//############################
func TestAddMetricSimple(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)
	// printMemStore(m)

	m.AddMetrics(key, 1584022800, testMetrics[0])
	m.AddMetrics(key, 1584022890, testMetrics[1])

	want := testMetrics[0][0].Value
	got := m.containers[key].current.store[0]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}

	want = testMetrics[1][2].Value
	got = m.containers[key].current.store[21]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}
	// printMemStore(m)
}

func TestAddMetricReplace(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)
	// printMemStore(m)

	m.AddMetrics(key, 1584022800, testMetrics[0])
	m.AddMetrics(key, 1584022800, testMetrics[1])

	want := testMetrics[1][0].Value
	got := m.containers[key].current.store[0]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}

	m.AddMetrics(key, 1584022850, testMetrics[0])
	want = testMetrics[0][0].Value
	got = m.containers[key].current.store[0]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}

	m.AddMetrics(key, 1584022860, testMetrics[1])
	want = testMetrics[0][0].Value
	got = m.containers[key].current.store[0]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}
	// printMemStore(m)
}

func TestAddMetricSwitch(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)
	// printMemStore(m)

	m.AddMetrics(key, 1584023000, testMetrics[0])
	m.AddMetrics(key, 1584023580, testMetrics[1])

	want := testMetrics[1][2].Value
	got := m.containers[key].current.store[29]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}

	m.AddMetrics(key, 1584023600, testMetrics[2])
	want = testMetrics[2][2].Value
	got = m.containers[key].current.store[20]
	if got != want {
		t.Errorf("Want %f got %f\n", want, got)
	}

	// printMemStore(m)
}

//############################
//#### Blackbox tests ########
//############################

func TestAddMetricOutOfBounds(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 30, 60)

	err := m.AddMetrics(key, 1584023000, testMetrics[0])
	if err != nil {
		t.Errorf("Got error 1584023000\n")
	}
	err = m.AddMetrics(key, 1584026600, testMetrics[0])
	if err == nil {
		t.Errorf("Got no error 1584026600\n")
	}
	err = m.AddMetrics(key, 1584021580, testMetrics[1])
	if err == nil {
		t.Errorf("Got no error 1584021580\n")
	}
	err = m.AddMetrics(key, 1584024580, testMetrics[1])
	if err != nil {
		t.Errorf("Got error 1584024580\n")
	}
	err = m.AddMetrics(key, 1584091580, testMetrics[1])
	if err == nil {
		t.Errorf("Got no error 1584091580\n")
	}
	err = m.AddMetrics(key, 1584024780, testMetrics[0])
	if err != nil {
		t.Errorf("Got error 1584024780\n")
	}
}

func TestGetMetricPlainCurrent(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}

	// printMemStore(m)
	val, tsFrom, err := m.GetMetric(key, "flops", 1584023000, 1584023560)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 9 {
		t.Errorf("Want 9. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}
	if val[8] != 970.5 {
		t.Errorf("Want 970.5 Got %f\n", val[9])
	}
}

func TestGetMetricPlainNext(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)
	val, tsFrom, err := m.GetMetric(key, "flops", 1584023000, 1584023560)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 9 {
		t.Errorf("Want 9. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}
	if val[8] != 970.5 {
		t.Errorf("Want 970.5 Got %f\n", val[9])
	}
}

func TestGetMetricGap(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*120), testMetrics[i])
	}

	val, tsFrom, err := m.GetMetric(key, "flops", 1584023000, 1584023600)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 10 {
		t.Errorf("Want 10. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}
	if !math.IsNaN(float64(val[1])) {
		t.Errorf("Want NaN Got %f\n", val[1])
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}

	// fmt.Println(val)
}

func TestGetMetricSplit(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584023200, 1584023860)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023200 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 11 {
		t.Errorf("Want 11. Got %d\n", len(val))
	}
	if val[0] != 940.5 {
		t.Errorf("Want 940.5 Got %f\n", val[0])
	}
	if val[10] != 950.5 {
		t.Errorf("Want 950.5 Got %f\n", val[0])
	}
}

func TestGetMetricExceedNext(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584022800, 1584023400)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 6 {
		t.Errorf("Want 6. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}
	if val[5] != 980.5 {
		t.Errorf("Want 980.5 Got %f\n", val[5])
	}
}

func TestGetMetricExceedNextSplit(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584022800, 1584023900)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 15 {
		t.Errorf("Want 14. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 100.5 Got %f\n", val[0])
	}
	if val[14] != 960.5 {
		t.Errorf("Want 960.5 Got %f\n", val[13])
	}
}

func TestGetMetricExceedCurrent(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584023800, 1584027900)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023800 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 7 {
		t.Errorf("Want 6. Got %d\n", len(val))
	}
	if val[0] != 950.5 {
		t.Errorf("Want 950.5 Got %f\n", val[0])
	}
	if val[6] != 930.5 {
		t.Errorf("Want 930.5 Got %f\n", val[5])
	}
}

func TestGetMetricExceedCurrentSplit(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584023120, 1584027900)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023120 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 18 {
		t.Errorf("Want 18. Got %d\n", len(val))
	}
	if val[0] != 980.5 {
		t.Errorf("Want 950.5 Got %f\n", val[0])
	}
	if val[17] != 930.5 {
		t.Errorf("Want 930.5 Got %f\n", val[17])
	}
}

func TestGetMetricExceedBoth(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	val, tsFrom, err := m.GetMetric(key, "flops", 1584022800, 1584027900)

	if err != nil {
		t.Errorf("Got error\n")
	}
	if tsFrom != 1584023000 {
		t.Errorf("Start ts differs: %d\n", tsFrom)
	}
	if len(val) != 20 {
		t.Errorf("Want 20. Got %d\n", len(val))
	}
	if val[0] != 100.5 {
		t.Errorf("Want 950.5 Got %f\n", val[0])
	}
	if val[19] != 930.5 {
		t.Errorf("Want 930.5 Got %f\n", val[17])
	}
}

func TestGetMetricOutUpper(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	_, _, err := m.GetMetric(key, "flops", 1584032800, 1584037900)

	if err == nil {
		t.Errorf("Got no error\n")
	}
}

func TestGetMetricOutLower(t *testing.T) {
	key := "m1220"
	m := newMemoryStore([]string{"flops", "mem_bw", "mem_capacity"}, 10, 60)

	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023000+i*60), testMetrics[i])
	}
	for i := 0; i < len(testMetrics); i++ {
		m.AddMetrics(key, int64(1584023600+i*60), testMetricsAlt[i])
	}

	// printMemStore(m)

	_, _, err := m.GetMetric(key, "flops", 1584002800, 1584007900)

	if err == nil {
		t.Errorf("Got no error\n")
	}
}
