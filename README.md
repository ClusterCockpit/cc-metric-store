# ClusterCockpit Metric Store

[![Build & Test](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-metric-store/actions/workflows/test.yml)

Barely unusable yet. Go look at the [GitHub Issues](https://github.com/ClusterCockpit/cc-metric-store/issues) for a progress overview.

### REST API Endpoints

_TODO... (For now, look at the examples below)_

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
The implementation calls those nodes `level` to avoid confusion. It is impossible to access data
only by knowing the *socket* or *cpu* tag, all higher up levels have to be specified as well.

Metrics have to be specified in advance! Those are taken from the *fields* of a line-protocol message.
New levels will be created on the fly at any depth, meaning that the clusters, hosts, sockets, number of cpus,
and so on do *not* have to be known at startup. Every level can hold all kinds of metrics. If a level is asked for
metrics it does not have itself, *all* child-levels will be asked for their values for that metric and
the data will be aggregated on a per-timestep basis.

A.t.m., there is no way to specify which CPU belongs to which Socket, so the hierarchy within a node is flat. That
will probably change.

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

### Config file

- `metrics`: Map of metric-name to objects with the following properties
    - `frequency`: Timestep/Interval/Resolution of this metric (In seconds)
    - `aggregation`: Can be `"sum"`, `"avg"` or `null`
        - `null` means aggregation across nodes is forbidden for this metric
        - `"sum"` means that values from the child levels are summed up for the parent level
        - `"avg"` means that values from the child levels are averaged for the parent level
    - `scope`: Unused at the moment, should be something like `"node"`, `"socket"` or `"cpu"`
- `nats`: Url of NATS.io server (The `updates` channel will be subscribed for metrics)
- `archive-root`: Directory to be used as archive
- `restore-last-hours`: After restart, load data from the past *X* hours back to memory
- `checkpoint-interval-hours`: Every *X* hours, write currently held data to disk

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

