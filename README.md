# ClusterCockpit Metric Store

Unusable yet. Go look at the [GitHub Issues](https://github.com/ClusterCockpit/cc-metric-store/issues) for a progress overview.

### REST API Endpoints

The following endpoints are implemented (not properly tested, subject to change):

- *from* and *to* need to be Unix timestamps in seconds
- *class* needs to be `node`, `socket` or `cpu` (The class of each metric is documented in [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector))
- If the *class* is `socket`, the hostname needs to be appended by `:s<socket-index>`. The same goes for `cpu` and `:c<cpu-index>`
- Fetch all datapoints from *from* to *to* for the hosts *h1*, *h2* and *h3* and metrics *m1* and *m2*
    - Request: `GET /api/<cluster>/<class>/<from>/<to>/timeseries?host=<h1>&host=<h2>&host=<h3>&metric=<m1>&metric=<m2>&...`
    - Response: `{ "m1": { "hosts": [{ "host": "h1", "start": <start-time>, "data": [1.0, 2.0, 3.0, ...] }, { "host": "h2" , ...}, { "host": "h3", ...}] }, ... }`
- Fetch the average, minimum and maximum values from *from* to *to* for the hosts *h1*, *h2* and *h3* and metrics *m1* and *m2*
    - Request: `GET /api/<cluster>/<class>/<from>/<to>/stats?host=<h1>&host=<h2>&host=<h3>&metric=<m1>&metric=<m2>&...`
    - Response: `{ "m1": { "hosts": [{ "host": "h1", "samples": 123, "avg": 0.5, "min": 0.0, "max": 1.0 }, ...] }, ... }`
    - `samples` is the number of actual measurements taken into account. This can be lower than expected if data ponts are missing
- Fetch the newest value for each host and metric on a specified cluster
    - Request: `GET /api/<cluster>/<class>/peak`
    - Response: `{ "host1": { "metric1": 1., "metric2": 2., ... }, "host2": { ... }, ... }`

### Run tests

```sh
# Test the line protocol parser
go test ./lineprotocol -v
# Test the memory store
go test . -v
```

### Test the complete setup (excluding ClusterCockpit itself)

First, get a NATS server running:

```sh
# Only needed once, downloads the docker image
docker pull nats:latest

# Start the NATS server
docker run -p 4222:4222 -ti nats:latest
```

Second, build and start start the [cc-metric-collector](https://github.com/ClusterCockpit/cc-metric-collector) using the following as `config.json`:

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

Third, build and start the metric store.

```sh
# Assuming you have a clone of this repo in ./cc-metric-store:
cd cc-metric-store
go get
go build
./cc-metric-store
```

Use this as `config.json`:

```json
{
    "metrics": {
        "node": {
            "frequency": 3,
            "metrics": ["load_five", "load_fifteen", "proc_total", "proc_run", "load_one"]
        },
        "socket": {
            "frequency": 3,
            "metrics": ["power", "mem_bw"]
        },
        "cpu": {
            "frequency": 3,
            "metrics": ["flops_sp", "flops_dp", "flops_any", "clock", "cpi"]
        }
    }
}
```

And finally, use the API to fetch some data:

```sh
# If the collector and store and nats-server have been running for at least 60 seconds on the same host, you may run:
curl -D - "http://localhost:8080/api/testcluster/node/$(expr $(date +%s) - 60)/$(date +%s)/timeseries?metric=load_one&host=$(hostname)"

# or:
curl -D - "http://localhost:8080/api/testcluster/socket/$(expr $(date +%s) - 60)/$(date +%s)/timeseries?metric=mem_bw&metric=power&host=$(hostname):s0"

# or:
curl -D - "http://localhost:8080/api/testcluster/cpu/$(expr $(date +%s) - 60)/$(date +%s)/timeseries?metric=flops_any&host=$(hostname):c0&host=$(hostname):c1"

# or:
curl -D - "http://localhost:8080/api/testcluster/node/peak"

# or:
curl -D - "http://localhost:8080/api/testcluster/socket/$(expr $(date +%s) - 60)/$(date +%s)/stats?metric=mem_bw&metric=power&host=$(hostname):s0"

# ...
```

