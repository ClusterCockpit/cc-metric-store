// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-metric-store/internal/api"
	"github.com/ClusterCockpit/cc-metric-store/internal/avro"
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/ClusterCockpit/cc-metric-store/internal/memorystore"
	"github.com/ClusterCockpit/cc-metric-store/internal/runtimeEnv"
	"github.com/google/gops/agent"
	httpSwagger "github.com/swaggo/http-swagger"
)

var (
	date    string
	commit  string
	version string
)

func main() {
	var configFile, flagLogLevel string
	var enableGopsAgent, flagVersion, flagDev, flagLogDateTime bool

	flag.StringVar(&configFile, "config", "./config.json", "configuration file")
	flag.StringVar(&flagLogLevel, "loglevel", "warn", "Sets the logging level: `[debug,info,warn (default),err,fatal,crit]`")
	flag.BoolVar(&flagLogDateTime, "logdate", false, "Set this flag to add date and time to log messages")
	flag.BoolVar(&enableGopsAgent, "gops", false, "Listen via github.com/google/gops/agent")
	flag.BoolVar(&flagDev, "dev", false, "Enable development Swagger UI component")
	flag.BoolVar(&flagVersion, "version", false, "Show version information and exit")
	flag.Parse()

	if flagVersion {
		fmt.Printf("Version:\t%s\n", version)
		fmt.Printf("Git hash:\t%s\n", commit)
		fmt.Printf("Build time:\t%s\n", date)
		os.Exit(0)
	}

	cclog.Init(flagLogLevel, flagLogDateTime)
	startupTime := time.Now()
	config.Init(configFile)
	memorystore.Init(config.Keys.Metrics)
	ms := memorystore.GetMemoryStore()

	if enableGopsAgent || config.Keys.Debug.EnableGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			cclog.Fatal(err)
		}
	}

	d, err := time.ParseDuration(config.Keys.Checkpoints.Restore)
	if err != nil {
		cclog.Fatalf("error parsing checkpoint restore duration: %v\n", err)
	}

	restoreFrom := startupTime.Add(-d)
	cclog.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := ms.FromCheckpointFiles(config.Keys.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := ms.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		cclog.Fatalf("loading checkpoints failed: %s\n", err.Error())
	} else {
		cclog.Infof("checkpoints loaded (%d files, %d MB, that took %fs)\n",
			files, loadedData, time.Since(startupTime).Seconds())
	}

	// Try to use less memory by forcing a GC run here and then
	// lowering the target percentage. The default of 100 means
	// that only once the ratio of new allocations execeds the
	// previously active heap, a GC is triggered.
	// Forcing a GC here will set the "previously active heap"
	// to a minumum.
	runtime.GC()
	if loadedData > 1000 && os.Getenv("GOGC") == "" {
		debug.SetGCPercent(10)
	}

	ctx, shutdown := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(4)

	memorystore.Retention(&wg, ctx)
	memorystore.Checkpointing(&wg, ctx)
	memorystore.Archiving(&wg, ctx)
	avro.DataStaging(&wg, ctx)

	r := http.NewServeMux()
	api.MountRoutes(r)

	if flagDev {
		cclog.Info("Enable Swagger UI!")
		r.HandleFunc("GET /swagger/", httpSwagger.Handler(
			httpSwagger.URL("http://"+config.Keys.HttpConfig.Address+"/swagger/doc.json")))
	}

	server := &http.Server{
		Handler:      r,
		Addr:         config.Keys.HttpConfig.Address,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	// Start http or https server
	listener, err := net.Listen("tcp", config.Keys.HttpConfig.Address)
	if err != nil {
		cclog.Fatalf("starting http listener failed: %v", err)
	}

	if config.Keys.HttpConfig.CertFile != "" && config.Keys.HttpConfig.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.Keys.HttpConfig.CertFile, config.Keys.HttpConfig.KeyFile)
		if err != nil {
			cclog.Fatalf("loading X509 keypair failed: %v", err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{cert},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		})
		fmt.Printf("HTTPS server listening at %s...\n", config.Keys.HttpConfig.Address)
	} else {
		fmt.Printf("HTTP server listening at %s...\n", config.Keys.HttpConfig.Address)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
			cclog.Fatalf("starting server failed: %v", err)
		}
	}()

	wg.Add(1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer wg.Done()
		<-sigs
		runtimeEnv.SystemdNotifiy(false, "Shutting down ...")
		server.Shutdown(context.Background())
		shutdown()
		memorystore.Shutdown()
	}()

	if config.Keys.Nats != nil {
		for _, natsConf := range config.Keys.Nats {
			// TODO: When multiple nats configs share a URL, do a single connect.
			wg.Add(1)
			nc := natsConf
			go func() {
				// err := ReceiveNats(conf.Nats, decodeLine, runtime.NumCPU()-1, ctx)
				err := api.ReceiveNats(nc, ms, 1, ctx)
				if err != nil {
					cclog.Fatal(err)
				}
				wg.Done()
			}()
		}
	}

	runtimeEnv.SystemdNotifiy(true, "running")
	wg.Wait()
	cclog.Info("Graceful shutdown completed!")
}
