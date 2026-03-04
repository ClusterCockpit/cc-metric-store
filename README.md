# ClusterCockpit Metric Store

[![Build & Test](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml)

The cc-metric-store provides a simple in-memory time series database for storing
metrics of cluster nodes at preconfigured intervals. It is meant to be used as
part of the [ClusterCockpit suite](https://github.com/ClusterCockpit). As all
data is kept in-memory, accessing it is very fast. It also provides topology aware
aggregations over time _and_ nodes/sockets/cpus.

There are major limitations: Data only gets written to disk at periodic
checkpoints (or via WAL on every write), not immediately as it is received.
Only the configured retention window is kept in memory.
Still metric data is kept as long as running jobs is using it.

The storage engine is provided by the
[cc-backend](https://github.com/ClusterCockpit/cc-backend) package
(`cc-backend/pkg/metricstore`). This repository provides the HTTP API wrapper.

The [NATS.io](https://nats.io/) based writing endpoint and the HTTP write
endpoint both consume messages in [this format of the InfluxDB line
protocol](https://github.com/ClusterCockpit/cc-specifications/blob/master/metrics/lineprotocol_alternative.md).

## Building

`cc-metric-store` can be built using the provided `Makefile`.
It supports the following targets:

- `make`: Build the application, copy an example configuration file and generate
  checkpoint folders if required.
- `make clean`: Clean the golang build cache and application binary
- `make distclean`: In addition to the clean target also remove the `./var`
  folder and `config.json`
- `make swagger`: Regenerate the Swagger files from the source comments.
- `make test`: Run tests and basic checks (`go build`, `go vet`, `go test`).

## Running

```sh
./cc-metric-store                              # Uses ./config.json
./cc-metric-store -config /path/to/config.json
./cc-metric-store -dev                         # Enable Swagger UI at /swagger/
./cc-metric-store -loglevel debug              # debug|info|warn (default)|err|crit
./cc-metric-store -logdate                     # Add date and time to log messages
./cc-metric-store -version                     # Show version information and exit
./cc-metric-store -gops                        # Enable gops agent for debugging
```

## REST API Endpoints

The REST API is documented in [swagger.json](./api/swagger.json). You can
explore and try the REST API using the integrated [SwaggerUI web
interface](http://localhost:8082/swagger/) (requires the `-dev` flag).

For more information on the `cc-metric-store` REST API have a look at the
ClusterCockpit documentation [website](https://clustercockpit.org/docs/reference/cc-metric-store/ccms-rest-api/).

All endpoints support both trailing-slash and non-trailing-slash variants:

| Method | Path                | Description                            |
| ------ | ------------------- | -------------------------------------- |
| `GET`  | `/api/query/`       | Query metrics with selectors           |
| `POST` | `/api/write/`       | Write metrics (InfluxDB line protocol) |
| `POST` | `/api/free/`        | Free buffers up to a timestamp         |
| `GET`  | `/api/debug/`       | Dump internal state                    |
| `GET`  | `/api/healthcheck/` | Check node health status               |

If `jwt-public-key` is set in `config.json`, all endpoints require JWT
authentication using an Ed25519 key (`Authorization: Bearer <token>`).

## Run tests

Some benchmarks concurrently access the `MemoryStore`, so enabling the
[Race Detector](https://golang.org/doc/articles/race_detector) might be useful.
The benchmarks also work as tests as they do check if the returned values are as
expected.

```sh
# Tests only
go test -v ./...

# Benchmarks as well
go test -bench=. -race -v ./...
```

## What are these selectors mentioned in the code?

The cc-metric-store works as a time-series database and uses the InfluxDB line
protocol as input format. Unlike InfluxDB, the data is indexed by one single
strictly hierarchical tree structure. A selector is built out of the tags in the
InfluxDB line protocol, and can be used to select a node (not in the sense of a
compute node, can also be a socket, cpu, ...) in that tree. The implementation
calls those nodes `level` to avoid confusion. It is impossible to access data
only by knowing the _socket_ or _cpu_ tag — all higher up levels have to be
specified as well.

This is what the hierarchy currently looks like:

- cluster1
  - host1
    - socket0
    - socket1
    - ...
    - cpu1
    - cpu2
    - cpu3
    - cpu4
    - ...
    - gpu1
    - gpu2
  - host2
  - ...
- cluster2
- ...

Example selectors:

1. `["cluster1", "host1", "cpu0"]`: Select only the cpu0 of host1 in cluster1
2. `["cluster1", "host1", ["cpu4", "cpu5", "cpu6", "cpu7"]]`: Select only CPUs 4-7 of host1 in cluster1
3. `["cluster1", "host1"]`: Select the complete node. If querying for a CPU-specific metric such as flops, all CPUs are implied

## Config file

The config file is a JSON document with four top-level sections.

### `main`

```json
"main": {
  "addr": "0.0.0.0:8082",
  "https-cert-file": "",
  "https-key-file": "",
  "jwt-public-key": "<base64-encoded Ed25519 public key>",
  "user": "",
  "group": "",
  "backend-url": ""
}
```

- `addr`: Address and port to listen on (default: `0.0.0.0:8082`)
- `https-cert-file` / `https-key-file`: Paths to TLS certificate/key for HTTPS
- `jwt-public-key`: Base64-encoded Ed25519 public key for JWT authentication. If empty, no auth is required.
- `user` / `group`: Drop privileges to this user/group after startup
- `backend-url`: Optional URL of a cc-backend instance used as node provider

### `metrics`

Per-metric configuration. Each key is the metric name:

```json
"metrics": {
  "cpu_load": { "frequency": 60, "aggregation": null },
  "flops_any": { "frequency": 60, "aggregation": "sum" },
  "cpu_user":  { "frequency": 60, "aggregation": "avg" }
}
```

- `frequency`: Sampling interval in seconds
- `aggregation`: How to aggregate sub-level data: `"sum"`, `"avg"`, or `null` (no aggregation)

### `metric-store`

```json
"metric-store": {
  "checkpoints": {
    "file-format": "wal",
    "directory": "./var/checkpoints"
  },
  "memory-cap": 100,
  "retention-in-memory": "24h",
  "num-workers": 0,
  "cleanup": {
    "mode": "archive",
    "directory": "./var/archive"
  },
  "nats-subscriptions": [
    { "subscribe-to": "hpc-nats", "cluster-tag": "fritz" }
  ]
}
```

- `checkpoints.file-format`: Checkpoint format: `"json"` (default, human-readable) or `"wal"` (binary WAL, crash-safe). See [Checkpoint formats](#checkpoint-formats) below.
- `checkpoints.directory`: Root directory for checkpoint files (organized as `<dir>/<cluster>/<host>/`)
- `memory-cap`: Approximate memory cap in MB for metric buffers
- `retention-in-memory`: How long to keep data in memory (e.g. `"48h"`)
- `num-workers`: Number of parallel workers for checkpoint/archive I/O (0 = auto, capped at 10)
- `cleanup.mode`: What to do with data older than `retention-in-memory`: `"archive"` (write Parquet) or `"delete"`
- `cleanup.directory`: Root directory for Parquet archive files (required when `mode` is `"archive"`)
- `nats-subscriptions`: List of NATS subjects to subscribe to, with associated cluster tag

### Checkpoint formats

The `checkpoints.file-format` field controls how in-memory data is persisted to disk.

**`"json"` (default)** — human-readable JSON snapshots written periodically. Each
snapshot is stored as `<dir>/<cluster>/<host>/<timestamp>.json` and contains the
full metric hierarchy. Easy to inspect and recover manually, but larger on disk
and slower to write.

**`"wal"`** — binary Write-Ahead Log format designed for crash safety. Two file
types are used per host:

- `current.wal` — append-only binary log. Every incoming data point is appended
  immediately (magic `0xCC1DA7A1`, 4-byte CRC32 per record). Truncated trailing
  records from unclean shutdowns are silently skipped on restart.
- `<timestamp>.bin` — binary snapshot written at each checkpoint interval
  (magic `0xCC5B0001`). Contains the complete hierarchical metric state
  column-by-column. Written atomically via a `.tmp` rename.

On startup the most recent `.bin` snapshot is loaded, then any remaining WAL
entries are replayed on top. The WAL is rotated (old file deleted, new one
started) after each successful snapshot.

The `"wal"` option is the default and will be the only supported option in the
future. The `"json"` checkpoint format is still provided to migrate from
previous cc-metric-store version.

### Parquet archive

When `cleanup.mode` is `"archive"`, data that ages out of the in-memory
retention window is written to [Apache Parquet](https://parquet.apache.org/)
files before being freed. Files are organized as:

```
<cleanup.directory>/
  <cluster>/
    <timestamp>.parquet
```

One Parquet file is produced per cluster per cleanup run, consolidating all
hosts. Rows use a long (tidy) schema:

| Column      | Type    | Description                                                             |
| ----------- | ------- | ----------------------------------------------------------------------- |
| `cluster`   | string  | Cluster name                                                            |
| `hostname`  | string  | Host name                                                               |
| `metric`    | string  | Metric name                                                             |
| `scope`     | string  | Hardware scope (`node`, `socket`, `core`, `hwthread`, `accelerator`, …) |
| `scope_id`  | string  | Numeric ID within the scope (e.g. `"0"`)                                |
| `timestamp` | int64   | Unix timestamp (seconds)                                                |
| `frequency` | int64   | Sampling interval in seconds                                            |
| `value`     | float32 | Metric value                                                            |

Files are compressed with Zstandard and sorted by `(cluster, hostname, metric,
timestamp)` for efficient columnar reads. The `cpu` prefix in the tree is
treated as an alias for `hwthread` scope.

### `nats`

```json
"nats": {
  "address": "nats://0.0.0.0:4222",
  "username": "root",
  "password": "root"
}
```

NATS connection is optional. If not configured, only the HTTP write endpoint is available.

For more information see the ClusterCockpit documentation [website](https://clustercockpit.org/docs/reference/cc-metric-store/ccms-configuration/).

## Test the complete setup (excluding cc-backend itself)

There are two ways for sending data to the cc-metric-store, both of which are
supported by the
[cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector).
This example uses NATS; the alternative is to use HTTP.

```sh
# Only needed once, downloads the docker image
docker pull nats:latest

# Start the NATS server
docker run -p 4222:4222 -ti nats:latest
```

Second, build and start the
[cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector)
using the following as Sink-Config:

```json
{
  "type": "nats",
  "host": "localhost",
  "port": "4222",
  "database": "updates"
}
```

Third, build and start the metric store. For this example here, the
`config.json` file already in the repository should work just fine.

```sh
# Assuming you have a clone of this repo in ./cc-metric-store:
cd cc-metric-store
make
./cc-metric-store
```

And finally, use the API to fetch some data. The API is protected by JWT based
authentication if `jwt-public-key` is set in `config.json`. You can use this JWT
for testing:
`eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw`

```sh
JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# If the collector and store and nats-server have been running for at least 60 seconds on the same host:
curl -H "Authorization: Bearer $JWT" \
     "http://localhost:8082/api/query/" \
     -d '{
       "cluster": "testcluster",
       "from": '"$(expr $(date +%s) - 60)"',
       "to": '"$(date +%s)"',
       "queries": [{ "metric": "cpu_load", "host": "'"$(hostname)"'" }]
     }'
```

For debugging, the debug endpoint dumps the current content to stdout:

```sh
JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# Dump everything
curl -H "Authorization: Bearer $JWT" "http://localhost:8082/api/debug/"

# Dump a specific selector (colon-separated path)
curl -H "Authorization: Bearer $JWT" "http://localhost:8082/api/debug/?selector=testcluster:host1"
```
