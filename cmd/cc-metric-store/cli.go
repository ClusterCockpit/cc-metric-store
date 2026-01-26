// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main provides the entry point for the ClusterCockpit metric store server.
// This file defines all command-line flags and their default values.
package main

import "flag"

var (
	flagGops, flagVersion, flagDev, flagLogDateTime bool
	flagConfigFile, flagLogLevel                    string
)

func cliInit() {
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.BoolVar(&flagDev, "dev", false, "Enable development component: Swagger UI")
	flag.BoolVar(&flagVersion, "version", false, "Show version information and exit")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug, info, warn (default), err, crit]`")
	flag.Parse()
}
