package avro

import (
	"fmt"
	"sync"

	"github.com/ClusterCockpit/cc-metric-store/internal/util"
)

var LineProtocolMessages = make(chan AvroStruct)

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

func (l *AvroLevel) addMetric(metricName string, value util.Float, timestamp int64) {
	l.lock.Lock()
	defer l.lock.Unlock()

	// Create a key value for the first time
	if len(l.data) == 0 {
		l.data[timestamp] = make(map[string]util.Float, 0)
		l.data[timestamp][metricName] = value
		fmt.Printf("Creating new timestamp because no data exists\n")
	}

	// Iterate over timestamps and choose the one which is within range.
	// Since its epoch time, we check if the difference is less than 60 seconds.
	for ts := range l.data {
		if (ts - timestamp) < 60 {
			l.data[ts][metricName] = value
			return
		}
	}

	// Create a new timestamp if none is found
	l.data[timestamp] = make(map[string]util.Float, 0)
	l.data[timestamp][metricName] = value

}
