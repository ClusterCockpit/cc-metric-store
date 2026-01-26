// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"log"
	"net/http"

	"github.com/ClusterCockpit/cc-metric-store/internal/config"
)

func MountRoutes(r *http.ServeMux) {
	if len(config.Keys.JwtPublicKey) > 0 {
		buf, err := base64.StdEncoding.DecodeString(config.Keys.JwtPublicKey)
		if err != nil {
			log.Fatalf("starting server failed: %v", err)
		}
		publicKey := ed25519.PublicKey(buf)
		// Compatibility
		r.Handle("POST /api/free", authHandler(http.HandlerFunc(freeMetrics), publicKey))
		r.Handle("POST /api/write", authHandler(http.HandlerFunc(writeMetrics), publicKey))
		r.Handle("GET /api/query", authHandler(http.HandlerFunc(handleQuery), publicKey))
		r.Handle("GET /api/debug", authHandler(http.HandlerFunc(debugMetrics), publicKey))
		r.Handle("GET /api/healthcheck", authHandler(http.HandlerFunc(metricsHealth), publicKey))
		// Refactor
		r.Handle("POST /api/free/", authHandler(http.HandlerFunc(freeMetrics), publicKey))
		r.Handle("POST /api/write/", authHandler(http.HandlerFunc(writeMetrics), publicKey))
		r.Handle("GET /api/query/", authHandler(http.HandlerFunc(handleQuery), publicKey))
		r.Handle("GET /api/debug/", authHandler(http.HandlerFunc(debugMetrics), publicKey))
		r.Handle("GET /api/healthcheck/", authHandler(http.HandlerFunc(metricsHealth), publicKey))
	} else {
		// Compatibility
		r.HandleFunc("POST /api/free", freeMetrics)
		r.HandleFunc("POST /api/write", writeMetrics)
		r.HandleFunc("GET /api/query", handleQuery)
		r.HandleFunc("GET /api/debug", debugMetrics)
		r.HandleFunc("GET /api/healthcheck", metricsHealth)
		// Refactor
		r.HandleFunc("POST /api/free/", freeMetrics)
		r.HandleFunc("POST /api/write/", writeMetrics)
		r.HandleFunc("GET /api/query/", handleQuery)
		r.HandleFunc("GET /api/debug/", debugMetrics)
		r.HandleFunc("GET /api/healthcheck/", metricsHealth)
	}
}
