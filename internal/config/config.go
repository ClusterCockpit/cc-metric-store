// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

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

type MetricConfig struct {
	// Interval in seconds at which measurements will arive.
	Frequency int64 `json:"frequency"`

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy `json:"aggregation"`

	// Private, used internally...
	Offset int
}

var metrics map[string]MetricConfig

type Config struct {
	Address  string `json:"addr"`
	CertFile string `json:"https-cert-file"`
	KeyFile  string `json:"https-key-file"`
	User     string `json:"user"`
	Group    string `json:"group"`
	Debug    struct {
		DumpToFile string `json:"dump-to-file"`
		EnableGops bool   `json:"gops"`
	} `json:"debug"`
	JwtPublicKey string `json:"jwt-public-key"`
}

var Keys Config

func InitMetrics(metricConfig json.RawMessage) {
	Validate(metricConfigSchema, metricConfig)
	dec := json.NewDecoder(bytes.NewReader(metricConfig))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&metrics); err != nil {
		cclog.Abortf("Config Init: Could not decode config file '%s'.\nError: %s\n", metricConfig, err.Error())
	}
}

func Init(mainConfig json.RawMessage) {
	Validate(configSchema, mainConfig)
	dec := json.NewDecoder(bytes.NewReader(mainConfig))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&Keys); err != nil {
		cclog.Abortf("Config Init: Could not decode config file '%s'.\nError: %s\n", mainConfig, err.Error())
	}
}

func GetMetricFrequency(metricName string) (int64, error) {
	if metric, ok := metrics[metricName]; ok {
		return metric.Frequency, nil
	}
	return 0, fmt.Errorf("metric %s not found", metricName)
}
