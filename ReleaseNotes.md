# `cc-metric-store` version 1.5.3

This is a bugfix release of `cc-metric-store`, the metric timeseries cache
implementation of ClusterCockpit. Since the storage engine is now part of
`cc-backend` we will follow the version number of `cc-backend`.
For release specific notes visit the [ClusterCockpit Documentation](https://clusterockpit.org/docs/release/).

## Notable changes

- **`-cleanup-checkpoints` CLI flag**: New flag triggers checkpoint cleanup
  (delete or archive to Parquet) based on the configured retention and cleanup
  settings, then exits. Useful for one-off maintenance without starting the full
  server.
- **GC initialised before checkpoint load**: `GOGC=15` is now set before
  `metricstore.Init` so the garbage-collector baseline is established prior to
  the largest allocation event (loading checkpoints from disk), reducing
  unnecessary heap growth at startup.
- **Dependency upgrades**: `cc-backend` updated from v1.5.0 to v1.5.3;
  `cc-lib` updated from v2.8.0 to v2.11.0; `nats.go` bumped from v1.49.0 to
  v1.50.0; `parquet-go` bumped from v0.28.0 to v0.29.0; various other module
  upgrades.

## Metricstore package fixes (cc-backend v1.5.0 → v1.5.3)

The following fixes landed in the upstream `cc-backend/pkg/metricstore` package
and are included via the dependency upgrade:

- **WAL correctness**: Fixed WAL rotation being skipped for all nodes due to a
  non-blocking send on a too-small channel; fixed unbound growth of WAL files
  when a checkpointing error occurs; fixed bugs in the WAL journal pipeline.
- **WAL throughput**: Sharded the WAL consumer for higher write throughput; added
  buffered I/O to WAL writes.
- **Checkpoint stability**: Paused WAL writes during binary checkpoint creation
  to prevent message drops; restructured cleanup archiving to stay within the
  32 k row limit of `parquet-go`.
- **Memory**: Fixed a memory explosion caused by broken emergency-free and batch
  aborts; reduced memory usage in the Parquet checkpoint archiver; fixed
  preventing memory spikes in the Parquet writer during the move/archive policy.
- **NATS**: Fixed blocking `ReceiveNats` call; fixed NATS contention under load.
- **Shutdown**: Increased shutdown timeouts; added WAL flush interval tuning;
  added shutdown timing logs.
- **Observability**: Added verbose logs for `DataDoesNotAlign` errors; reduced
  noise by demoting a missing-metric warning to debug level.
- **Configuration**: Restored `checkpointInterval` as an optional config key.

## Breaking changes (from v1.4.x)

- The internal `memorystore`, `avro`, `resampler`, and `util` packages have been
  removed. The storage engine is now provided by the
  [`cc-backend`](https://github.com/ClusterCockpit/cc-backend) package
  (`cc-backend/pkg/metricstore`). This repository is now the HTTP API wrapper
  only.
- The configuration schema has changed. Refer to `configs/config.json` for the
  updated structure.
