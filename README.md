# ClusterCockpit Metric Store

[![Build & Test](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml)

The cc-metric-store provides a simple in-memory time series database for storing metrics of cluster nodes at preconfigured intervals. It is meant to be used as part of the [ClusterCockpit suite](https://github.com/ClusterCockpit). As all data is kept in-memory (but written to disk as compressed JSON for long term storage), accessing it is very fast. It also provides aggregations over time *and* nodes/sockets/cpus.

There are major limitations: Data only gets written to disk at periodic checkpoints, not as soon as it is received.

Go look at the `TODO.md` file and the [GitHub Issues](https://github.com/ClusterCockpit/cc-metric-store/issues) for a progress overview. Things work, but are not properly tested.
The [NATS.io](https://nats.io/) based writing endpoint consumes messages in [this format of the InfluxDB line protocol](https://github.com/ClusterCockpit/cc-specifications/blob/master/metrics/lineprotocol_alternative.md).

### REST API Endpoints

The REST API is documented in [openapi.yaml](./openapi.yaml) in the OpenAPI 3.0 format.

### Run tests

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

### What are these selectors mentioned in the code?

Tags in InfluxDB are used to build indexes over the stored data. InfluxDB-Tags have no
relation to each other, they do not depend on each other and have no hierarchy.
Different tags build up different indexes (I am no expert at all, but this is how i think they work).

This project also works as a time-series database and uses the InfluxDB line protocol.
Unlike InfluxDB, the data is indexed by one single strictly hierarchical tree structure.
A selector is build out of the tags in the InfluxDB line protocol, and can be used to select
a node (not in the sense of a compute node, can also be a socket, cpu, ...) in that tree.
The implementation calls those nodes `level` to avoid confusion.
It is impossible to access data only by knowing the *socket* or *cpu* tag, all higher up levels have to be specified as well.

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
  - host2
  - ...
- cluster2
- ...

Example selectors:
1. `["cluster1", "host1", "cpu0"]`: Select only the cpu0 of host1 in cluster1
2. `["cluster1", "host1", ["cpu4", "cpu5", "cpu6", "cpu7"]]`: Select only CPUs 4-7 of host1 in cluster1
3. `["cluster1", "host1"]`: Select the complete node. If querying for a CPU-specific metric such as floats, all CPUs are implied

### Config file

All durations are specified as string that will be parsed [like this](https://pkg.go.dev/time#ParseDuration) (Allowed suffixes: `s`, `m`, `h`, ...).

- `metrics`: Map of metric-name to objects with the following properties
    - `frequency`: Timestep/Interval/Resolution of this metric
    - `aggregation`: Can be `"sum"`, `"avg"` or `null`
        - `null` means aggregation across nodes is forbidden for this metric
        - `"sum"` means that values from the child levels are summed up for the parent level
        - `"avg"` means that values from the child levels are averaged for the parent level
    - `scope`: Unused at the moment, should be something like `"node"`, `"socket"` or `"hwthread"`
- `nats`: Url of NATS.io server (The `updates` channel will be subscribed for metrics), example: "nats://localhost:4222"
- `http-api-address`: Where to listen via HTTP, example: ":8080"
- `jwt-public-key`: Base64 encoded string, use this to verify requests to the HTTP API
- `retention-on-memory`: Keep all values in memory for at least that amount of time
- `checkpoints`:
    - `interval`: Do checkpoints every X seconds/minutes/hours
    - `directory`: Path to a directory
    - `restore`: After a restart, load the last X seconds/minutes/hours of data back into memory

- `archive`:
    - `interval`: Move and compress all checkpoints not needed anymore every X seconds/minutes/hours
    - `directory`: Path to a directory

### Test the complete setup (excluding ClusterCockpit itself)

There are two ways for sending data to the cc-metric-store, both of which are supported by the [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector). This example uses Nats, the alternative is to use HTTP.

```sh
# Only needed once, downloads the docker image
docker pull nats:latest

# Start the NATS server
docker run -p 4222:4222 -ti nats:latest
```

Second, build and start the [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector) using the following as Sink-Config:

```json
{
  "type": "nats",
  "host": "localhost",
  "port": "4222",
  "database": "updates"
}
```

Third, build and start the metric store. For this example here, the `config.json` file
already in the repository should work just fine.

```sh
# Assuming you have a clone of this repo in ./cc-metric-store:
cd cc-metric-store
go get
go build
./cc-metric-store
```

And finally, use the API to fetch some data. The API is protected by JWT based authentication if `jwt-public-key` is set in `config.json`. You can use this JWT for testing: `eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw`

```sh
JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# If the collector and store and nats-server have been running for at least 60 seconds on the same host, you may run:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/query" -d "{ \"cluster\": \"testcluster\", \"from\": $(expr $(date +%s) - 60), \"to\": $(date +%s), \"queries\": [{
  \"metric\": \"load_one\",
  \"host\": \"$(hostname)\"
}] }"

# ...
```

