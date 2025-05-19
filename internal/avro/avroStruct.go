package avro

import (
	"sync"

	"github.com/ClusterCockpit/cc-metric-store/internal/util"
)

var LineProtocolMessages = make(chan AvroStruct)
var Delimiter = "ZZZZZ"
var AvroCounter = 0

// CheckpointBufferMinutes should always be in minutes.
// Its controls the amount of data to hold for given amount of time.
var CheckpointBufferMinutes = 3

type AvroStruct struct {
	MetricName string
	Cluster    string
	Node       string
	Selector   []string
	Value      util.Float
	Timestamp  int64
}

type AvroStore struct {
	root AvroLevel
}

var avroStore AvroStore

type AvroLevel struct {
	children map[string]*AvroLevel
	data     map[int64]map[string]util.Float
	lock     sync.RWMutex
}

type AvroField struct {
	Name    string      `json:"name"`
	Type    interface{} `json:"type"`
	Default interface{} `json:"default,omitempty"`
}

type AvroSchema struct {
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	Fields []AvroField `json:"fields"`
}

func (l *AvroLevel) findAvroLevelOrCreate(selector []string) *AvroLevel {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	var child *AvroLevel
	var ok bool
	if l.children == nil {
		// Children map needs to be created...
		l.lock.RUnlock()
	} else {
		child, ok := l.children[selector[0]]
		l.lock.RUnlock()
		if ok {
			return child.findAvroLevelOrCreate(selector[1:])
		}
	}

	// The level does not exist, take write lock for unqiue access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	if l.children != nil {
		child, ok = l.children[selector[0]]
		if ok {
			l.lock.Unlock()
			return child.findAvroLevelOrCreate(selector[1:])
		}
	}

	child = &AvroLevel{
		data:     make(map[int64]map[string]util.Float, 0),
		children: nil,
	}

	if l.children != nil {
		l.children[selector[0]] = child
	} else {
		l.children = map[string]*AvroLevel{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findAvroLevelOrCreate(selector[1:])
}

func (l *AvroLevel) addMetric(metricName string, value util.Float, timestamp int64, Freq int) {
	l.lock.Lock()
	defer l.lock.Unlock()

	KeyCounter := int(CheckpointBufferMinutes * 60 / Freq)

	// Create keys in advance for the given amount of time
	if len(l.data) != KeyCounter {
		if len(l.data) == 0 {
			for i := range KeyCounter {
				l.data[timestamp+int64(i*Freq)] = make(map[string]util.Float, 0)
			}
		} else {
			//Get the last timestamp
			var lastTs int64
			for ts := range l.data {
				if ts > lastTs {
					lastTs = ts
				}
			}
			// Create keys for the next KeyCounter timestamps
			l.data[lastTs+int64(Freq)] = make(map[string]util.Float, 0)
		}
	}

	// Iterate over timestamps and choose the one which is within range.
	// Since its epoch time, we check if the difference is less than 60 seconds.
	for ts := range l.data {
		if _, ok := l.data[ts][metricName]; ok {
			// If the metric is already present, we can skip it
			continue
		}
		if (ts - timestamp) < int64(Freq) {
			l.data[ts][metricName] = value
			break
		}
	}
}

func GetAvroStore() *AvroStore {
	return &avroStore
}
