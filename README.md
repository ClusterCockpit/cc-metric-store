# ClusterCockpit Metric Store

[![Build & Test](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml)

Go look at the `TODO.md` file and the [GitHub Issues](https://github.com/ClusterCockpit/cc-metric-store/issues) for a progress overview. Things work, but are not properly tested.
The [NATS.io](https://nats.io/) based writing endpoint consumes messages in [this format of the InfluxDB line protocol](https://github.com/ClusterCockpit/cc-specifications/blob/master/metrics/lineprotocol_alternative.md).

### REST API Endpoints

In case `jwt-public-key` is a non-empty string in the `config.json` file, the API is protected by JWT based authentication. The signing algorithm has to be `Ed25519`, but no
fields are required in the JWT payload. Expiration will be checked if specified. The JWT has to be provided using the HTTP `Authorization` header.

All but one endpoints use *selectors* to access the data. A selector must be an array of strings or another array of strings. Examples are provided below.

In the requests, `to` and `from` have to be UNIX timestamps in seconds. The response might also contain `from`/`to` timestamps. They can differ from those in the request,
if there was not data for a section of the requested data.

1. `POST /api/<from>/<to>/timeseries`
    - Request-Body: `{ "selectors": [<sel1>, <sel2>, <sel3>, ...], "metrics": ["flops_any", "load_one", ...] }`
    - The response will be a JSON array, each entry in the array corresponding to the selector found at that index in the request's `selectors` array
    - Each array entry will be a map from every requested metric to this: `{ "from": Timestamp, "to": Timestamp, "data": Array of Floats }`
    - Some values in `data` might be `null` if there is no data available for that time slot
2. `POST /api/<from>/<to>/stats`
    - The Request-Body shall be the same as for a `timeseries` query
    - The response will be a JSON array, each entry in the array corresponding to the selector found at that index in the request's `selectors` array
    - Each array entry will be a map from every requested metric to this: `{ "from": Timestamp, "to": Timestamp, "samples": Int, "avg": Float, "min": Float, "max": Float }`
    - If the `samples` value is 0, the statistics should be ignored.
3. `POST /api/<to>/free`
    - Request-Body: Array of selectors
    - This request will free up and release all data older than `to` for all nodes specified by the selectors
4. `GET /api/{cluster}/peek`
    - Return a map from every node in the specified cluster to a map from every metric to the newest value available for that metric
    - All cpu/socket level metrics are aggregated to the node level
5. `POST /api/write`
    - You can send lines of the InfluxDB line protocol to this endpoint and they will be written to the store (Basically an alternative to NATS)

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

### What are these selectors mentioned in the code and API?

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

All durations are specified in seconds.

- `metrics`: Map of metric-name to objects with the following properties
    - `frequency`: Timestep/Interval/Resolution of this metric (In seconds)
    - `aggregation`: Can be `"sum"`, `"avg"` or `null`
        - `null` means aggregation across nodes is forbidden for this metric
        - `"sum"` means that values from the child levels are summed up for the parent level
        - `"avg"` means that values from the child levels are averaged for the parent level
    - `scope`: Unused at the moment, should be something like `"node"`, `"socket"` or `"cpu"`
- `nats`: Url of NATS.io server (The `updates` channel will be subscribed for metrics), example: "nats://localhost:4222"
- `http-api-address`: Where to listen via HTTP, example: ":8080"
- `jwt-public-key`: Base64 encoded string, use this to verify requests to the HTTP API
- `retention-on-memory`: Keep all values in memory for at least that amount of seconds

### Test the complete setup (excluding ClusterCockpit itself)

First, get a NATS server running:

```sh
# Only needed once, downloads the docker image
docker pull nats:latest

# Start the NATS server
docker run -p 4222:4222 -ti nats:latest
```

Second, build and start the [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector) using the following as `config.json`:

```json
{
    "sink": {
        "type": "nats",
        "host": "localhost",
        "port": "4222",
        "database": "updates"
    },
    "interval" : 3,
    "duration" : 1,
    "collectors": [ "likwid", "loadavg" ],
    "default_tags": { "cluster": "testcluster" },
    "receiver": { "type": "none" }
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
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/timeseries" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\"]], \"metrics\": [\"load_one\"] }"

# Get flops_any for all CPUs:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/timeseries" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\"]], \"metrics\": [\"flops_any\"] }"

# Get flops_any for CPU 0:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/timeseries" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\", \"cpu0\"]], \"metrics\": [\"flops_any\"] }"

# Get flops_any for CPU 0, 1, 2 and 3:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/timeseries" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\", [\"cpu0\", \"cpu1\", \"cpu2\", \"cpu3\"]]], \"metrics\": [\"flops_any\"] }"

# Stats for load_one and proc_run:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/stats" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\"]], \"metrics\": [\"load_one\", \"proc_run\"] }"

# Stats for *all* CPUs aggregated both from CPU to node and over time:
curl -H "Authorization: Bearer $JWT" -D - "http://localhost:8080/api/$(expr $(date +%s) - 60)/$(date +%s)/stats" -d "{ \"selectors\": [[\"testcluster\", \"$(hostname)\"]], \"metrics\": [\"flops_sp\", \"flops_dp\"] }"


# ...
```

