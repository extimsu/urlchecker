# urlchecker

urlchecker checks whether hosts answer over TCP, UDP or ICMP (ping). It works as a one-shot CLI and as a Prometheus exporter. In exporter mode it pings ICMP targets continuously and exports the same counters as [smokeping_prober](https://github.com/SuperQ/smokeping_prober), so you can alert on packet loss.

Contents:

- [Quick start](#quick-start): a first check and a first exporter.
- [How-to guides](#how-to-guides): recipes for common jobs.
- [Reference](#reference): targets, flags, config file and metrics.
- [Explanation](#explanation): how the checks work and what they do not tell you.
- [Helm chart](charts/urlchecker/README.md): every value of the Kubernetes chart.

## Quick start

You need Go 1.23 or later, or Docker. To run it on Kubernetes, see [Deploy on Kubernetes with the Helm chart](#deploy-on-kubernetes-with-the-helm-chart).

1. Build the binary:

   ```console
   make build
   ```

2. Check one host. The default protocol is TCP and the default port is 80:

   ```console
   $ ./bin/urlchecker --url example.com
   🟢 [+] [tcp]  example.com:80 (0.031s)
   ```

   `[+]` means the connection was accepted. `[-]` means it failed. The colour shows the response time against `--warn-threshold` and `--crit-threshold`.

3. Run the exporter for a TCP port and a ping target:

   ```console
   sudo ./bin/urlchecker --exporter --url example.com:443,icmp://1.1.1.1
   ```

4. Open <http://localhost:9090/metrics>. You will see `urlchecker_current_status` for `example.com:443` and `urlchecker_icmp_requests_total` for `1.1.1.1`.

`sudo` lets the exporter open a raw ICMP socket. To run without root, see [Run ICMP without root](#run-icmp-without-root).

## How-to guides

### Check several hosts at once

Separate targets with commas. A target without a port uses `--port`:

```console
./urlchecker --url example.com,example.org:443 --port 8080 --timeout 3s
```

Or put one target per line in a file:

```console
./urlchecker --file urls.txt
```

### Get JSON output for scripts

```console
./urlchecker --file urls.txt --json
```

You get one JSON object per target, then a summary with group health.

### Check a UDP port

```console
./urlchecker --url udp://10.0.0.53:53
```

Or set `--protocol udp` for every target without a prefix. A UDP check sends no data, so read [What a UDP check proves](#what-a-udp-check-proves) first.

### Measure packet loss to hosts

1. List the hosts as `icmp://` targets. You can mix them with TCP targets:

   ```text
   # targets.txt
   icmp://10.0.50.101
   icmp://10.0.50.102
   10.0.50.101:10250
   10.0.50.102:10250
   ```

2. Start the exporter:

   ```console
   urlchecker --exporter --file targets.txt --metrics-port 9375 --ping-interval 1s --timeout 2s
   ```

3. In Prometheus, compute the loss per target over 5 minutes:

   ```promql
   1 - (
     rate(urlchecker_icmp_response_duration_seconds_count[5m])
       / rate(urlchecker_icmp_requests_total[5m])
   )
   ```

   `0` means no loss and `1` means every ping was lost. A target that never replies still shows `1`, because its reply series is created at `0` when the pinger starts.

4. For the average round-trip time, divide `rate(urlchecker_icmp_response_duration_seconds_sum[5m])` by the `_count` rate. For percentiles, use `histogram_quantile` on the `_bucket` series.

### Run ICMP without root

urlchecker tries a raw ICMP socket first, then the unprivileged ping socket. Use one of these:

- **Linux, unprivileged socket.** The process group ID must be inside `net.ipv4.ping_group_range`. Docker sets this to `0 2147483647` by default, so the image's `nonroot` user can ping.
- **Raw socket without full root.** Give the process `CAP_NET_RAW`.

In Kubernetes, the setup depends on the pod's network:

- **Pod network.** Set the safe sysctl `net.ipv4.ping_group_range` on the pod to its own group, for example `"65532 65532"`. The non-root user can then ping, and the pod still passes the `restricted` Pod Security Standard. The Helm chart does this by default.
- **`hostNetwork: true`.** Pod sysctls can't change the node's network namespace, and many nodes set `ping_group_range` to `1 0`, which disables the unprivileged socket. Run as UID 0 with only `NET_RAW` instead. The chart does this when `icmp.rawSocket=true`:

```yaml
securityContext:
  runAsUser: 0
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ALL]
    add: [NET_RAW]
```

If neither socket can be opened, urlchecker logs `open ICMP socket: raw: ...; unprivileged: ...` and tries again on every interval.

### Run the exporter in Docker

```console
docker run -d --name urlchecker -p 9090:9090 \
  docker.io/extim/urlchecker:0.2.0 \
  --exporter --url example.com:443,icmp://1.1.1.1 --workers 3 --check-interval 30s
```

### Deploy on Kubernetes with the Helm chart

The chart lives in [`charts/urlchecker`](charts/urlchecker) and is published to
`oci://ghcr.io/extimsu/charts/urlchecker`.

1. Check endpoints from inside the cluster. This runs a non-root Deployment:

   ```console
   helm install probe oci://ghcr.io/extimsu/charts/urlchecker \
     -n monitoring --create-namespace \
     --set 'targets={api.example.com:443,icmp://10.0.0.1}' \
     --set serviceMonitor.enabled=true
   ```

2. To measure packet loss between nodes, run one pod per node on the host
   network. List every node's IP as an `icmp://` target:

   ```yaml
   # values-nodes.yaml
   workload: { kind: DaemonSet }
   hostNetwork: true
   icmp: { rawSocket: true }      # UID 0 with only NET_RAW
   tolerations: [{ operator: Exists }]
   metrics: { port: 9375 }        # must be free on the nodes
   service: { port: 9375 }
   targets: [icmp://10.0.0.11, icmp://10.0.0.12, icmp://10.0.0.13]
   ```

   ```console
   helm install node-probes oci://ghcr.io/extimsu/charts/urlchecker -n monitoring -f values-nodes.yaml
   ```

3. Check the release:

   ```console
   helm test probe -n monitoring
   ```

The [chart README](charts/urlchecker/README.md) lists every value. It also
covers ICMP privileges in each mode and Pod Security levels.

### Graph the metrics in Grafana

The dashboard [`charts/urlchecker/dashboards/urlchecker.json`](charts/urlchecker/dashboards/urlchecker.json)
covers the exporter's metrics:

- target status and p95 response time;
- status over time and failed checks;
- ICMP loss, round trip and a smokeping-style round-trip heatmap;
- a loss matrix of instance × ICMP target.

It needs Grafana 10 or later and a Prometheus-compatible data source.

- **Import it by hand.** In Grafana, open **Dashboards → New → Import**, upload
  the file, and pick your data source.
- **Ship it with the Helm chart.** Set `grafanaDashboard.enabled=true`. The chart
  creates a ConfigMap labelled `grafana_dashboard: "1"`, which the Grafana
  sidecar of the grafana and kube-prometheus-stack charts loads. If the sidecar
  watches only its own namespace, set `grafanaDashboard.namespace` to that
  namespace.

Use the filters to pick the data source, job, instance and targets. The
dashboard has no panels for retries, circuit breakers or groups, because the
exporter does not emit those metrics. The one-shot and metrics modes do.

### Use a config file

```console
./urlchecker --config example-config.yaml
```

[`example-config.yaml`](example-config.yaml) lists every key. See [Config file](#config-file) for how it combines with flags.

## Reference

### Targets

| Form | Protocol | Example |
|------|----------|---------|
| `host` | `--protocol`, port `--port` | `example.com` |
| `host:port` | `--protocol` | `example.com:443` |
| `tcp://host:port` | TCP | `tcp://10.0.0.1:22` |
| `udp://host:port` | UDP | `udp://10.0.0.53:53` |
| `icmp://host` | ICMP echo (IPv4) | `icmp://10.0.0.1` |

The prefix wins over `--protocol`, so one run can mix protocols. Targets come from `--url` (comma-separated), from `--file`, or from `urls` and `groups` in the config file.

The target file has one target per line. It also accepts:

- Lines starting with `#`, which are comments.
- Empty lines.
- `[group:<name>]` headers, which put the targets below them into a group.

`--group` puts every target from the file into one group.

### Modes

| Mode | Flag | Behaviour |
|------|------|-----------|
| One-shot | none | Checks every target once, prints the results and exits. ICMP sends one echo request. |
| Metrics | `--metrics` | Repeats the one-shot check every `--check-interval` and serves metrics. ICMP sends one echo per interval and sets `urlchecker_current_status`. |
| Exporter | `--exporter` | A pool of `--workers` checks the TCP and UDP targets every `--check-interval`. Each ICMP target gets its own pinger, which sends one echo every `--ping-interval` and exports the `urlchecker_icmp_*` metrics. |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--url` | | Targets, comma-separated |
| `--file` | | File with one target per line |
| `--config` | | YAML or JSON config file |
| `--port` | `80` | Port for targets without one |
| `--protocol` | `tcp` | Protocol for targets without a prefix: `tcp`, `udp` or `icmp` |
| `--timeout` | `5s` | Connect timeout, or wait for an echo reply. In exporter mode the ping timeout is capped at `--ping-interval`. |
| `--json` | `false` | JSON output |
| `--metrics` | `false` | Metrics mode |
| `--exporter` | `false` | Exporter mode |
| `--metrics-port` | `9090` | Port for `/metrics` |
| `--check-interval` | `30s` | Interval for TCP and UDP checks, and for ICMP in metrics mode |
| `--ping-interval` | `1s` | Interval between echo requests per ICMP target in exporter mode |
| `--workers` | `5` | Worker count for exporter mode |
| `--group` | | Group name for all targets |
| `--warn-threshold` | `500ms` | Response time shown as 🟡 |
| `--crit-threshold` | `1s` | Response time shown as 🔴 |
| `--retry-count` | `3` | Retries after a failed check (one-shot and metrics modes) |
| `--retry-delay` | `1s` | First retry delay, doubled each retry |
| `--circuit-threshold` | `5` | Failures before the circuit breaker opens |
| `--circuit-timeout` | `60s` | Time before an open circuit breaker is tried again |
| `--version` | `false` | Print the version |

### Config file

The config keys match the flags in snake_case: `urls`, `file`, `port`, `protocol`, `timeout`, `json_output`, `metrics`, `metrics_port`, `exporter`, `check_interval`, `workers`, `ping_interval`, `group_name`, `groups`, `warning_threshold`, `critical_threshold`, `retry_count`, `retry_delay`, `circuit_breaker_threshold` and `circuit_breaker_timeout`.

Flags win over the file. Known limitation: a flag's default value also wins over the file. Set a value as a flag if the file's value is ignored. File values work for `urls`, `groups` and the boolean switches.

### Metrics

TCP and UDP checks, and ICMP in one-shot and metrics modes, use the labels `url` and `protocol`:

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_current_status` | gauge | `1` if the last check passed, else `0` |
| `urlchecker_total_checks` | counter | Checks run |
| `urlchecker_failed_checks` | counter | Checks failed |
| `urlchecker_response_time_seconds` | histogram | Check response time |
| `urlchecker_check_duration_seconds` | histogram | Check duration including retries |
| `urlchecker_retry_attempts_total` | counter | Retries |
| `urlchecker_circuit_breaker_state` | gauge | `0` closed, `1` half-open, `2` open |
| `urlchecker_circuit_breaker_failure_count` | gauge | Consecutive failures |
| `urlchecker_circuit_breaker_transitions_total` | counter | State changes, with label `transition` |

Group metrics use the label `group`:

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_group_health` | gauge | `1` if every target in the group passed |
| `urlchecker_group_total_urls` | gauge | Targets in the group |
| `urlchecker_group_healthy_urls` | gauge | Targets that passed |

ICMP in exporter mode uses the labels `host` (as written in the target) and `ip` (the resolved address):

| Metric | Type | smokeping_prober equivalent |
|--------|------|-----------------------------|
| `urlchecker_icmp_requests_total` | counter | `smokeping_requests_total` |
| `urlchecker_icmp_response_duration_seconds` | histogram, buckets 50µs × 2ⁿ up to ~26s | `smokeping_response_duration_seconds` |

## Explanation

### How a check works

A TCP check opens a connection and closes it right away. It passes when the handshake completes within `--timeout`, so it proves something is listening. It does not check what the service returns.

An ICMP check sends one echo request and waits for the reply that matches its ID and sequence number. It proves the host's network stack answers, not that any service is running.

### Why ICMP has its own loop

Packet loss of a few percent is invisible to a check that runs every 30 seconds. One lost packet out of two is a coin toss, and a TCP retransmit hides most of it anyway. To see 1% loss you need many samples, so the exporter pings each ICMP target every `--ping-interval`, one second by default. It counts every request and every reply. This is how smokeping_prober works, and the metric layout is the same, so existing loss queries need only the metric names changed.

The pinger waits for one reply before sending the next request. A reply that comes after the timeout counts as lost.

### Raw and unprivileged ICMP sockets

A raw ICMP socket needs `CAP_NET_RAW`, and it receives a copy of every ICMP packet that reaches the host. Each pinger has its own socket and filters by ID, sequence number and source address.

The unprivileged ping socket (Linux `SOCK_DGRAM`) needs no capability, but `net.ipv4.ping_group_range` must cover the process group. On Linux the kernel sets the echo ID and delivers only the matching replies.

urlchecker tries the raw socket first, so it works in both setups without a flag.

### What a UDP check proves

UDP has no handshake, and urlchecker sends no payload. A UDP check passes as soon as the name resolves and a route exists, even if nothing listens on the port. Use it to catch DNS and routing failures only, and use ICMP or TCP to test reachability.

### Limits

- ICMP is IPv4 only.
- There is one socket per ICMP target. That's fine for tens of targets. A shared socket would scale better to hundreds.
- Exporter mode does not retry TCP or UDP checks and has no circuit breaker. Each interval is a single attempt.

## License

[MIT](LICENSE)
