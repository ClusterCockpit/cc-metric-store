# `cc-metric-store` version 1.5.0

This is a major release of `cc-metric-store`, the metric timeseries cache
implementation of ClusterCockpit. Since the storage engine is now part of
`cc-backend` we will follow the version number of `cc-backend`.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Breaking changes

- The internal `memorystore`, `avro`, `resampler`, and `util` packages have been
  removed. The storage engine is now provided by the
  [`cc-backend`](https://github.com/ClusterCockpit/cc-backend) package
  (`cc-backend/pkg/metricstore`). This repository is now the HTTP API wrapper
  only.
- The configuration schema has changed. Refer to `configs/config.json` for the
  updated structure.

## Notable changes

- **Storage engine extracted to `cc-backend` library**: The entire in-memory
  time-series storage engine was moved to `cc-backend/pkg/metricstore`. This
  reduces duplication in the ClusterCockpit suite and enables shared maintenance
  of the storage layer.
- **HealthCheck API endpoint**: New `GET /api/healthcheck/` endpoint reports the
  health status of cluster nodes.
- **Dynamic memory management**: Memory limits can now be adjusted at runtime via
  a callback from the `cc-backend` library.
- **Configuration schema validation**: The config and metric config JSON schemas
  have been updated and are now validated against the structs they describe.
- **Startup refactored**: Application startup has been split into `cli.go` and
  `server.go` for clearer separation of concerns.
- **`go fix` applied**: Codebase updated to current Go idioms.
- **Dependency upgrades**: `nats.go` bumped from 1.36.0 to 1.47.0;
  `cc-lib` updated to v2.8.0; `cc-backend` updated to v1.5.0; various other
  module upgrades.
