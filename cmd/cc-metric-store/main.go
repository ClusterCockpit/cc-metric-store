// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
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
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/google/gops/agent"
)

var (
	date    string
	commit  string
	version string
)

func printVersion() {
	fmt.Printf("Version:\t%s\n", version)
	fmt.Printf("Git hash:\t%s\n", commit)
	fmt.Printf("Build time:\t%s\n", date)
}

func initGops() error {
	if !flagGops && !config.Keys.Debug.EnableGops {
		return nil
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		return fmt.Errorf("starting gops agent: %w", err)
	}
	return nil
}

func initConfiguration() error {
	ccconf.Init(flagConfigFile)

	cfg := ccconf.GetPackageConfig("main")
	if cfg == nil {
		return fmt.Errorf("main configuration must be present")
	}

	config.Init(cfg)
	return nil
}

func initSubsystems() error {
	// Initialize nats client
	natsConfig := ccconf.GetPackageConfig("nats")
	if err := nats.Init(natsConfig); err != nil {
		cclog.Warnf("initializing (optional) nats client: %s", err.Error())
	}
	nats.Connect()

	return nil
}

func runServer(ctx context.Context) error {
	var wg sync.WaitGroup

	// Initialize metric store if configuration is provided
	mscfg := ccconf.GetPackageConfig("metric-store")
	if mscfg != nil {
		metricstore.Init(mscfg, &wg)
	} else {
		return fmt.Errorf("missing metricstore configuration")
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
	cliInit()

	if flagVersion {
		printVersion()
		return nil
	}

	// Initialize logger
	cclog.Init(flagLogLevel, flagLogDateTime)

	// Initialize gops agent
	if err := initGops(); err != nil {
		return err
	}

	// Initialize subsystems in dependency order:
	// 1. Load configuration from config.json
	// 2. Initialize subsystems like nats

	// Load configuration
	if err := initConfiguration(); err != nil {
		return err
	}

	// Initialize subsystems (nats, etc.)
	if err := initSubsystems(); err != nil {
		return err
	}

	// Run server with context
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
