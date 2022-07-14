package types

import (
	"encoding/json"
	"fmt"
)

type Stats struct {
	Samples int   `json:"samples"`
	Min     Float `json:"min"`
	Avg     Float `json:"avg"`
	Max     Float `json:"max"`
}

type MetricConfig struct {
	// Interval in seconds at which measurements will arive.
	Frequency int64 `json:"frequency"`

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy `json:"aggregation"`

	// Private, used internally...
	Offset int
}

type Metric struct {
	Name  string
	Value Float
	Conf  MetricConfig
}

// For aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func (as *AggregationStrategy) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "":
		*as = NoAggregation
	case "sum":
		*as = SumAggregation
	case "avg":
		*as = AvgAggregation
	default:
		return fmt.Errorf("invalid aggregation strategy: %#v", str)
	}
	return nil
}
