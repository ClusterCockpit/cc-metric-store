// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
)

var metrics map[string]metricstore.MetricConfig

type Config struct {
	Address    string `json:"addr"`
	CertFile   string `json:"https-cert-file"`
	KeyFile    string `json:"https-key-file"`
	User       string `json:"user"`
	Group      string `json:"group"`
	BackendURL string `json:"backend-url"`
	Debug      struct {
		DumpToFile string `json:"dump-to-file"`
		EnableGops bool   `json:"gops"`
	} `json:"debug"`
	JwtPublicKey string `json:"jwt-public-key"`
}

var Keys Config

type metricConfigJSON struct {
	Frequency   int64  `json:"frequency"`
	Aggregation string `json:"aggregation"`
}

func InitMetrics(metricConfig json.RawMessage) {
	Validate(metricConfigSchema, metricConfig)

	var tempMetrics map[string]metricConfigJSON
	dec := json.NewDecoder(bytes.NewReader(metricConfig))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tempMetrics); err != nil {
		cclog.Abortf("Config Init: Could not decode config file '%s'.\nError: %s\n", metricConfig, err.Error())
	}

	metrics = make(map[string]metricstore.MetricConfig)
	for name, cfg := range tempMetrics {
		agg, err := metricstore.AssignAggregationStrategy(cfg.Aggregation)
		if err != nil {
			cclog.Warnf("Could not parse aggregation strategy for metric '%s': %s", name, err.Error())
		}
		metrics[name] = metricstore.MetricConfig{
			Frequency:   cfg.Frequency,
			Aggregation: agg,
		}
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

func GetMetrics() map[string]metricstore.MetricConfig {
	return metrics
}
