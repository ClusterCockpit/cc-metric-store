// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	ccconf "github.com/ClusterCockpit/cc-lib/v2/ccConfig"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/runtime"
	"github.com/ClusterCockpit/cc-metric-store/internal/api"
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/google/gops/agent"
)

var (
	date    string
	commit  string
	version string
)

var (
	flagGops, flagVersion, flagDev, flagLogDateTime bool
	flagConfigFile, flagLogLevel                    string
)

func printVersion() {
	fmt.Printf("Version:\t%s\n", version)
	fmt.Printf("Git hash:\t%s\n", commit)
	fmt.Printf("Build time:\t%s\n", date)
}

func runServer(ctx context.Context) error {
	var wg sync.WaitGroup

	mscfg := ccconf.GetPackageConfig("metrics")
	if mscfg == nil {
		return fmt.Errorf("missing metrics configuration")
	}
	config.InitMetrics(mscfg)

	mscfg = ccconf.GetPackageConfig("metric-store")
	if mscfg == nil {
		return fmt.Errorf("missing metricstore configuration")
	}

	metricstore.Init(mscfg, config.GetMetrics(), &wg)

	if config.Keys.BackendURL != "" {
		ms := metricstore.GetMemoryStore()
		ms.SetNodeProvider(api.NewBackendNodeProvider(config.Keys.BackendURL))
		cclog.Infof("Node provider configured with backend URL: %s", config.Keys.BackendURL)
	}

	// Initialize HTTP server
	srv, err := NewServer(version, commit, date)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	// Channel to collect errors from server
	errChan := make(chan error, 1)

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Handle shutdown signals
	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		select {
		case <-sigs:
			cclog.Info("Shutdown signal received")
		case <-ctx.Done():
		}

		runtime.SystemdNotify(false, "Shutting down ...")
		srv.Shutdown(ctx)
	}()

	runtime.SystemdNotify(true, "running")

	// Wait for completion or errors
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Wait for either server startup error or shutdown completion
	if err := <-errChan; err != nil {
		return err
	}

	cclog.Print("Graceful shutdown completed!")
	return nil
}

func run() error {
	flag.BoolVar(&flagGops, "gops", false, "Listen via github.com/google/gops/agent (for debugging)")
	flag.BoolVar(&flagDev, "dev", false, "Enable development component: Swagger UI")
	flag.BoolVar(&flagVersion, "version", false, "Show version information and exit")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.StringVar(&flagConfigFile, "config", "./config.json", "Specify alternative path to `config.json`")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug, info, warn (default), err, crit]`")
	flag.Parse()

	if flagVersion {
		printVersion()
		return nil
	}

	cclog.Init(flagLogLevel, flagLogDateTime)

	if flagGops || config.Keys.Debug.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			return fmt.Errorf("starting gops agent: %w", err)
		}
	}

	ccconf.Init(flagConfigFile)

	cfg := ccconf.GetPackageConfig("main")
	if cfg == nil {
		return fmt.Errorf("main configuration must be present")
	}

	config.Init(cfg)

	natsConfig := ccconf.GetPackageConfig("nats")
	if err := nats.Init(natsConfig); err != nil {
		cclog.Warnf("initializing (optional) nats client: %s", err.Error())
	}
	nats.Connect()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return runServer(ctx)
}

func main() {
	if err := run(); err != nil {
		cclog.Error(err.Error())
		os.Exit(1)
	}
}
