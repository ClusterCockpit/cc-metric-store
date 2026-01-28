// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

var metricConfigSchema = `
{
  "type": "object",
  "description": "Map of metric names to their configuration.",
  "additionalProperties": {
    "type": "object",
    "properties": {
      "frequency": {
        "description": "Sampling frequency in seconds.",
        "type": "integer"
      },
      "aggregation": {
        "description": "Aggregation strategy: 'sum', 'avg', or 'null'.",
        "type": "string"
      }
    },
    "required": ["frequency", "aggregation"]
  }
}`
