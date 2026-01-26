// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main provides the entry point for the ClusterCockpit metric store server.
// This file contains HTTP server setup, routing configuration.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/metricstore"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/nats"
	"github.com/ClusterCockpit/cc-lib/v2/runtime"
	"github.com/ClusterCockpit/cc-metric-store/internal/api"
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	httpSwagger "github.com/swaggo/http-swagger"
)

// Server encapsulates the HTTP server state and dependencies
type Server struct {
	router *http.ServeMux
	server *http.Server
}

// NewServer creates and initializes a new Server instance
func NewServer(version, commit, buildDate string) (*Server, error) {
	s := &Server{
		router: http.NewServeMux(),
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) init() error {
	api.MountRoutes(s.router)

	if flagDev {
		cclog.Print("Enable Swagger UI!")
		s.router.HandleFunc("GET /swagger/", httpSwagger.Handler(
			httpSwagger.URL("http://"+config.Keys.Address+"/swagger/doc.json")))
	}

	return nil
}

// Server timeout defaults (in seconds)
const (
	defaultReadTimeout  = 30
	defaultWriteTimeout = 30
)

func (s *Server) Start(ctx context.Context) error {
	// Use configurable timeouts with defaults
	readTimeout := time.Duration(defaultReadTimeout) * time.Second
	writeTimeout := time.Duration(defaultWriteTimeout) * time.Second

	s.server = &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Handler:      s.router,
		Addr:         config.Keys.Address,
	}

	// Start http or https server
	listener, err := net.Listen("tcp", config.Keys.Address)
	if err != nil {
		return fmt.Errorf("starting listener on '%s': %w", config.Keys.Address, err)
	}

	if config.Keys.CertFile != "" && config.Keys.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(
			config.Keys.CertFile, config.Keys.KeyFile)
		if err != nil {
			return fmt.Errorf("loading X509 keypair (check 'https-cert-file' and 'https-key-file' in config.json): %w", err)
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
		cclog.Infof("HTTPS server listening at %s...", config.Keys.Address)
	} else {
		cclog.Infof("HTTP server listening at %s...", config.Keys.Address)
	}

	// Because this program will want to bind to a privileged port (like 80), the listener must
	// be established first, then the user can be changed, and after that,
	// the actual http server can be started.
	if err := runtime.DropPrivileges(config.Keys.Group, config.Keys.User); err != nil {
		return fmt.Errorf("dropping privileges: %w", err)
	}

	// Handle context cancellation for graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			cclog.Errorf("Server shutdown error: %v", err)
		}
	}()

	if err = s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nc := nats.GetClient()
	if nc != nil {
		nc.Close()
	}

	// First shut down the server gracefully (waiting for all ongoing requests)
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		cclog.Errorf("Server shutdown error: %v", err)
	}

	// Archive all the metric store data
	metricstore.Shutdown()
}
