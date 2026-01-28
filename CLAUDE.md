# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

cc-metric-store is an in-memory time-series database for HPC cluster metrics, part of the ClusterCockpit monitoring suite. Data is indexed by a hierarchical tree (cluster → host → socket/cpu/gpu) and accessed via selectors. The core storage engine lives in `cc-backend/pkg/metricstore`; this repo provides the HTTP API wrapper.

## Build Commands

```bash
make                 # Build binary, copy config template, create checkpoint dirs
make clean           # Clean build cache and binary
make distclean       # Also remove ./var and config.json
make swagger         # Regenerate Swagger from source comments
make test            # Run go build, go vet, go test
```

## Testing

```bash
go test -v ./...              # Run tests
go test -bench=. -race -v ./...  # With benchmarks and race detector
```

Integration test scripts in `/endpoint-test-scripts/` for manual API testing.

## Running

```bash
./cc-metric-store                           # Uses ./config.json
./cc-metric-store -config /path/to/config.json
./cc-metric-store -dev                      # Enable Swagger UI at /swagger/
./cc-metric-store -loglevel debug           # debug|info|warn|err|crit
```

## Architecture

**Entry point:** `cmd/cc-metric-store/main.go`
- `run()` → parse flags, init logging/config, connect NATS
- `runServer()` → init metricstore from cc-backend, start HTTP server

**Key packages:**
- `internal/api/` - REST endpoints (query, write, free, debug, healthcheck) and JWT auth (Ed25519)
- `internal/config/` - Config loading and JSON schema validation
- External: `cc-backend/pkg/metricstore` - actual time-series storage engine

**API endpoints** (all support optional JWT auth):
- `GET /api/query/` - Query metrics with selectors
- `POST /api/write/` - Write metrics (InfluxDB line protocol)
- `POST /api/free/` - Free buffers up to timestamp
- `GET /api/debug/` - Dump internal state
- `GET /api/healthcheck/` - Node health status

## Selectors

Data is accessed via hierarchical selectors:
```
["cluster1", "host1", "cpu0"]           # Specific CPU
["cluster1", "host1", ["cpu4", "cpu5"]] # Multiple CPUs
["cluster1", "host1"]                   # Entire node (all CPUs implied)
```

## Configuration

Config file structure (see `configs/config.json`):
- `main` - Server address, TLS certs, JWT public key, user/group for privilege drop
- `metrics` - Per-metric frequency and aggregation strategy (sum/avg/null)
- `metric-store` - Checkpoints, memory cap, retention, cleanup mode, NATS subscriptions
- `nats` - Optional NATS connection for receiving metrics

## Test JWT

For testing with JWT auth enabled:
```
eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw
```
