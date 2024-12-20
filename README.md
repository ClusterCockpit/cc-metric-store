# ClusterCockpit Metric Store

[![Build & Test](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml)

The cc-metric-store provides a simple in-memory time series database for storing
metrics of cluster nodes at preconfigured intervals. It is meant to be used as
part of the [ClusterCockpit suite](https://github.com/ClusterCockpit). As all
data is kept in-memory (but written to disk as compressed JSON for long term
storage), accessing it is very fast. It also provides topology aware
aggregations over time _and_ nodes/sockets/cpus.

There are major limitations: Data only gets written to disk at periodic
checkpoints, not as soon as it is received. Also only the fixed configured
duration is stored and available.

Go look at the [GitHub
Issues](https://github.com/ClusterCockpit/cc-metric-store/issues) for a progress
overview. The [NATS.io](https://nats.io/) based writing endpoint consumes messages in [this
format of the InfluxDB line
protocol](https://github.com/ClusterCockpit/cc-specifications/blob/master/metrics/lineprotocol_alternative.md).

## Building

`cc-metric-store` can be built using the provided `Makefile`.
It supports the following targets:

- `make`: Build the application, copy a example configuration file and generate
  checkpoint folders if required.
- `make clean`: Clean the golang build cache and application binary
- `make distclean`: In addition to the clean target also remove the `./var`
  folder
- `make swagger`: Regenerate the Swagger files from the source comments.
- `make test`: Run test and basic checks.

## REST API Endpoints

The REST API is documented in [swagger.json](./api/swagger.json). You can
explore and try the REST API using the integrated [SwaggerUI web
interface](http://localhost:8082/swagger).

For more information on the `cc-metric-store` REST API have a look at the
ClusterCockpit documentation [website](https://clustercockpit.org/docs/reference/cc-metric-store/ccms-rest-api/)

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
strictly hierarchical tree structure. A selector is build out of the tags in the
InfluxDB line protocol, and can be used to select a node (not in the sense of a
compute node, can also be a socket, cpu, ...) in that tree. The implementation
calls those nodes `level` to avoid confusion. It is impossible to access data
only by knowing the _socket_ or _cpu_ tag, all higher up levels have to be
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
3. `["cluster1", "host1"]`: Select the complete node. If querying for a CPU-specific metric such as floats, all CPUs are implied

## Config file

You find the configuration options on the ClusterCockpit [website](https://clustercockpit.org/docs/reference/cc-metric-store/ccms-configuration/).

## Test the complete setup (excluding cc-backend itself)

There are two ways for sending data to the cc-metric-store, both of which are
supported by the
[cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector).
This example uses NATS, the alternative is to use HTTP.

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

# If the collector and store and nats-server have been running for at least 60 seconds on the same host, you may run:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/query" -d "{ \"cluster\": \"testcluster\", \"from\": $(expr $(date +%s) - 60), \"to\": $(date +%s), \"queries\": [{
  \"metric\": \"load_one\",
  \"host\": \"$(hostname)\"
}] }"

# ...
```

For debugging there is a debug endpoint to dump the current content to stdout:

```sh
JWT="eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw"

# If the collector and store and nats-server have been running for at least 60 seconds on the same host, you may run:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/debug"

# ...
```
