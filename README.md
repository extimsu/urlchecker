# urlchecker

urlchecker checks whether targets answer over TCP, UDP, ICMP (ping), HTTP(S) or DNS. It works as a one-shot CLI and as a Prometheus exporter.

In exporter mode it pings ICMP targets continuously and exports the same counters as [smokeping_prober](https://github.com/SuperQ/smokeping_prober), so you can alert on packet loss. It also times every phase of an HTTP request and reports TLS certificate expiry. In Kubernetes it can find the nodes itself and measure loss between every pair.

Contents:

- [Quick start](#quick-start): a first check and a first exporter.
- [How-to guides](#how-to-guides): recipes for common jobs.
- [Reference](#reference): target syntax, options, flags, config file, endpoints and metrics.
- [Explanation](#explanation): how the checks work and what they do not tell you.
- [Helm chart](charts/urlchecker/README.md): every value of the Kubernetes chart.
- [Changelog](CHANGELOG.md).

## Quick start

1. Get urlchecker. Use one of these:

   - A binary for Linux, macOS or Windows from the [releases page](https://github.com/extimsu/urlchecker/releases).
   - The image `ghcr.io/extimsu/urlchecker`. It is multi-arch and signed with cosign.
   - A build from source, which needs Go 1.26 or later:

     ```console
     make build
     ```

2. Check a few targets. A target without a protocol is TCP, and one without a port uses port 80:

   ```console
   $ ./bin/urlchecker --url 'example.com:443,https://example.com/,dns:///example.com'
   🟢 [+] [tcp]  example.com:443 (0.031s)
   🟢 [+] [https]  https://example.com/ (0.192s) HTTP 200
   🟢 [+] [dns]  dns:///example.com (0.053s) [172.66.147.243 104.20.23.154]
   ```

   `[+]` means the check passed, and `[-]` means it failed. The colour shows the response time against `--warn-threshold` and `--crit-threshold`. The exit code is 0 when every target passed and 1 when one failed.

3. Run the exporter for a web page and a ping target:

   ```console
   sudo ./bin/urlchecker --exporter --url 'https://example.com/,icmp://1.1.1.1'
   ```

4. Open <http://localhost:9090/metrics>:
   - `urlchecker_current_status` and `urlchecker_http_phase_duration_seconds` cover `https://example.com/`.
   - `urlchecker_icmp_requests_total` covers `1.1.1.1`.

`sudo` lets the exporter open a raw ICMP socket. To run without root, see [Run ICMP without root](#run-icmp-without-root).

## How-to guides

### Check several targets at once

Separate targets with commas. A target without a port uses `--port`:

```console
./urlchecker --url example.com,example.org:443 --port 8080 --timeout 3s
```

Or put one target per line in a file:

```console
./urlchecker --file targets.txt
```

### Use urlchecker in a script

```console
./urlchecker --file targets.txt --json --retry-count 1
```

You get one JSON object per target, in the order of the file, and then a summary with group health. The exit code tells the script the result:

- 0: every target passed.
- 1: at least one target failed.
- 2: a flag, the config file or a target line was wrong.

### Check an HTTP endpoint

```console
./urlchecker --url 'https://api.example.com/health status=200 body="\"status\":\s*\"ok\""'
```

- **Pass condition.** By default a status from 200 to 399 passes. Set `status` to `200`, `2xx`, `200-299` or `200,301`.
- **Body.** `body` is a regular expression that must match the first MiB of the body.
- **Redirects.** They are followed (up to 10); `follow=false` checks the redirect itself.
- **Request.** `method=HEAD` changes the method, and `host=api.internal` sets the `Host` header.

Each check uses a new connection, so the DNS, connect and TLS times are measured every time.

### Watch TLS certificate expiry

Every `https://` target exports `urlchecker_tls_cert_expiry_timestamp_seconds`: when the first certificate in the server's chain expires. This alert fires two weeks ahead:

```yaml
- alert: TLSCertExpiringSoon
  expr: urlchecker_tls_cert_expiry_timestamp_seconds - time() < 14 * 86400
  for: 1h
```

The expiry is recorded even when the certificate is already expired or untrusted: the check fails, and the metric still says when the certificate expired. `insecure=true` skips verification for internal endpoints with self-signed certificates.

### Check DNS resolution

```text
dns:///example.com                          # the system resolver, A record
dns://10.0.0.53/example.com type=AAAA       # this resolver, IPv6 addresses
dns://1.1.1.1/example.com type=MX expect=mail\.example\.com
```

A check fails on an error such as NXDOMAIN, on no answers, or when no answer matches `expect`. `urlchecker_dns_answers` counts the answers.

### Check a UDP port

```console
./urlchecker --url udp://10.0.0.53:53
```

Or set `--protocol udp` for every target without a prefix. A UDP check sends no data, so read [What a UDP check proves](#what-a-udp-check-proves) first. For a DNS server, a `dns://` check is better.

### Measure packet loss to hosts

1. List the hosts as `icmp://` targets. You can mix them with other targets:

   ```text
   # targets.txt
   icmp://10.0.50.101
   icmp://10.0.50.102
   10.0.50.101:10250
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

### Find MTU problems on a path

A normal ping is 64 bytes, so it gets through a path whose MTU is too small for real traffic. Send full-size pings with don't-fragment set instead (Linux only):

```text
icmp://10.0.50.101 size=1472 df=true
```

1472 bytes of payload plus 28 bytes of headers is a 1500-byte packet. If a link on the path has a smaller MTU, the echo is dropped instead of fragmented, and the target shows 100% loss. To ping the same host at the normal size too, list it under another name, such as its host name.

### Ping over IPv6

IPv6 addresses work as they are: `icmp://2001:db8::1`, `[2001:db8::1]:443`. A host name resolves to IPv4 first. Add `ip=6` to use its IPv6 address, or `ip=4` to force IPv4. The same option applies to TCP, UDP, HTTP and DNS targets.

### Label targets

Add a Prometheus-style label block to a target:

```text
icmp://10.0.50.13 {node="worker-03", zone="fsn1-dc14"}
https://api.example.com/health {team="api"}
```

The labels go on `urlchecker_target_info{protocol, target, ...} 1`, not on every series. Join them where you need them. `target` equals the `url` label of the check metrics and the `host` label of the ICMP metrics:

```promql
(urlchecker_current_status == 0)
  * on (instance, url) group_left (team)
    label_replace(urlchecker_target_info, "url", "$1", "target", "(.*)")
```

Join on `instance` as well: every urlchecker instance exports its own `urlchecker_target_info`.

### Change targets without a restart

The exporter reloads its config and target files in two cases:

- **On SIGHUP.**
- **When the files' content changes.** It checks every `--reload-interval`, 30 seconds by default.

Only added and removed targets start or stop. Removed targets' series are deleted, and added targets are checked at once.

A file that fails to load keeps the current targets. It also sets `urlchecker_config_last_reload_successful` to 0 and logs the file and line. In Kubernetes, edit the ConfigMap: the kubelet syncs it into the pod within about a minute.

### Run ICMP without root

urlchecker tries a raw ICMP socket first, then the unprivileged ping socket. Use one of these:

- **Linux, unprivileged socket.** The process group ID must be inside `net.ipv4.ping_group_range`. Docker sets this to `0 2147483647` by default, so the image's `nonroot` user can ping. The same sysctl covers IPv6.
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

If neither socket can be opened, the target is reported down with `open ICMP socket: raw: ...; unprivileged: ...`, and urlchecker tries again on every interval.

### Run the exporter in Docker

```console
docker run -d --name urlchecker -p 9090:9090 \
  ghcr.io/extimsu/urlchecker:0.3.0 \
  --exporter --url 'https://example.com/,icmp://1.1.1.1' --check-interval 30s
```

`docker.io/extim/urlchecker` has the same images.

### Deploy on Kubernetes with the Helm chart

The chart lives in [`charts/urlchecker`](charts/urlchecker) and is published to `oci://ghcr.io/extimsu/charts/urlchecker`. To check endpoints from inside the cluster, install a non-root Deployment:

```console
helm install probe oci://ghcr.io/extimsu/charts/urlchecker \
  -n monitoring --create-namespace \
  --set 'targets={https://api.example.com/health,icmp://10.0.0.1}' \
  --set serviceMonitor.enabled=true --set prometheusRule.enabled=true
helm test probe -n monitoring
```

The [chart README](charts/urlchecker/README.md) lists every value. It also covers ICMP privileges in each mode, Pod Security levels, alert rules and the Grafana dashboard.

### Measure loss between every Kubernetes node

Run one pod per node on the host network, and let each pod find the nodes through the Kubernetes API:

```yaml
# values-nodes.yaml
workload: { kind: DaemonSet }
hostNetwork: true
icmp: { rawSocket: true }        # UID 0 with only NET_RAW
tolerations: [{ operator: Exists }]
metrics: { port: 9375 }          # must be free on the nodes
service: { port: 9375 }
discovery:
  nodes:
    enabled: true                # ClusterRole with list on nodes
    tcpPorts: [10250]            # also check the kubelet port
```

```console
helm install node-probes oci://ghcr.io/extimsu/charts/urlchecker -n monitoring -f values-nodes.yaml
```

Each pod pings every other node's InternalIP every second. It picks up new nodes and drops removed ones within `discovery.nodes.interval`. The probed node's name is the `target_node` label on `urlchecker_target_info`.

Outside the chart, the same thing takes three settings:

- `--discover-nodes icmp,tcp:10250`;
- `--discover-exclude-self` with `NODE_NAME` set from `spec.nodeName`;
- a service account that may list nodes.

### Graph the metrics in Grafana

The dashboard [`charts/urlchecker/dashboards/urlchecker.json`](charts/urlchecker/dashboards/urlchecker.json) shows:

- **Checks:** target status, response time and failures for every protocol.
- **HTTP:** status codes, request phases and certificate expiry.
- **DNS:** the number of answers.
- **ICMP:**
  - loss and round trip;
  - a smokeping-style round-trip heatmap;
  - a loss matrix of instance × target.
- **The exporter:** reload, discovery and version state.

It needs Grafana 10 or later and a Prometheus-compatible data source.

- **Import it by hand.** In Grafana, open **Dashboards → New → Import**, upload the file, and pick your data source.
- **Ship it with the Helm chart.** Set `grafanaDashboard.enabled=true`. The chart creates a ConfigMap labelled `grafana_dashboard: "1"`, which the Grafana sidecar of the grafana and kube-prometheus-stack charts loads.

The JSON is generated by [`hack/gen-dashboard.py`](hack/gen-dashboard.py). Edit the script, not the JSON.

### Use a config file

```console
./urlchecker --config example-config.yaml
```

[`example-config.yaml`](example-config.yaml) shows the keys. Flags given on the command line win over the file. Flags you don't give leave the file's values alone.

### Verify a release

Images, the chart and the checksum file are signed with cosign keyless signatures from GitHub Actions:

```console
cosign verify ghcr.io/extimsu/urlchecker:0.3.0 \
  --certificate-identity-regexp 'https://github.com/extimsu/urlchecker/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/extimsu/urlchecker/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Each archive also has an SPDX SBOM on the release page.

## Reference

### Target line

```text
<target> [option=value ...] [{label="value", ...}]
```

| Target | Protocol | Example |
|--------|----------|---------|
| `host` | `--protocol`, port `--port` | `example.com` |
| `host:port` | `--protocol` | `example.com:443`, `[2001:db8::1]:443` |
| `tcp://host:port` | TCP connect | `tcp://10.0.0.1:22` |
| `udp://host:port` | UDP "connect" | `udp://10.0.0.53:53` |
| `icmp://host` | ICMP echo, IPv4 or IPv6 | `icmp://10.0.0.1`, `icmp://2001:db8::1` |
| `http://…`, `https://…` | HTTP request | `https://example.com/health?full=1` |
| `dns://[resolver[:port]]/name` | DNS lookup | `dns:///example.com`, `dns://10.0.0.53/example.com` |

Option values with spaces go in double quotes. Inside quotes, `\"` is a quote and `\\` a backslash; any other backslash stays as written, so regular expressions work. Labels follow Prometheus rules: names match `[a-zA-Z_][a-zA-Z0-9_]*`, and values are double-quoted. `protocol`, `target` and names starting with `__` are reserved.

Targets come from `--url` (comma-separated; commas inside quotes or `{}` don't split), from `--file`, and from `urls` and `groups` in the config file. A target listed twice is an error. The target file also accepts `#` comments, empty lines, and `[group:<name>]` headers that put the targets below them into a group.

### Options

| Option | Protocols | Default | Meaning |
|--------|-----------|---------|---------|
| `timeout` | all | `--timeout` | Timeout for this target |
| `ip` | all | IPv4 first | `4` or `6`: address family to resolve and connect with |
| `size` | icmp | 10 bytes | Echo payload in bytes, 0 to 65000 |
| `df` | icmp | `false` | Set don't-fragment (Linux) |
| `method` | http(s) | `GET` | Request method |
| `status` | http(s) | `200-399` | Passing status codes: `200`, `2xx`, `200-299`, or a comma list |
| `body` | http(s) | | Regular expression the first MiB of the body must match |
| `follow` | http(s) | `true` | `false` checks the redirect instead of following it |
| `insecure` | http(s) | `false` | Skip TLS verification |
| `host` | http(s) | | `Host` header |
| `type` | dns | `A` | `A`, `AAAA`, `CNAME`, `MX`, `NS`, `TXT`, `SRV` or `PTR` |
| `expect` | dns | | Regular expression one answer must match |

### Modes

| Mode | Flag | Behaviour |
|------|------|-----------|
| One-shot | none | Checks every target once, with retries, prints the results and exits. |
| Metrics | `--metrics` | Repeats the one-shot check every `--check-interval` and serves metrics. |
| Exporter | `--exporter` | A pool of `--workers` checks the TCP, UDP, HTTP and DNS targets every `--check-interval`, without retries. Each ICMP target gets its own pinger, which sends one echo every `--ping-interval`. Reloads its files and can discover Kubernetes nodes. |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Every target passed (one-shot), or a clean shutdown |
| 1 | At least one target failed (one-shot) |
| 2 | Bad flags, config file or target line; or the metrics port couldn't be opened |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--url` | | Targets, comma-separated |
| `--file` | | File with one target per line |
| `--config` | | YAML or JSON config file |
| `--port` | `80` | Port for targets without one |
| `--protocol` | `tcp` | Protocol for targets without a prefix: `tcp`, `udp`, `icmp`, `http`, `https` or `dns` |
| `--timeout` | `5s` | Timeout of a check. In exporter mode the ping timeout is capped at `--ping-interval`. |
| `--json` | `false` | JSON output |
| `--metrics` | `false` | Metrics mode |
| `--exporter` | `false` | Exporter mode |
| `--metrics-port` | `9090` | Port for `/metrics`, `/healthz` and `/readyz` |
| `--check-interval` | `30s` | Interval between checks of a TCP, UDP, HTTP or DNS target |
| `--ping-interval` | `1s` | Interval between echo requests per ICMP target (exporter) |
| `--workers` | `5` | Concurrent checks (exporter) |
| `--reload-interval` | `30s` | How often to check the files for changes (exporter); `0` reloads only on SIGHUP |
| `--resolve-interval` | `1m` | How often a pinger resolves its host name again (exporter) |
| `--discover-nodes` | | Check every Kubernetes node: `icmp`, `tcp:<port>`, or both (exporter) |
| `--discover-exclude-self` | `false` | Skip the node in `$NODE_NAME` |
| `--discovery-interval` | `30s` | How often to list the nodes |
| `--group` | | Group name for all targets |
| `--warn-threshold` | `500ms` | Response time shown as 🟡 |
| `--crit-threshold` | `1s` | Response time shown as 🔴 |
| `--retry-count` | `3` | Retries after a failed check (one-shot and metrics modes) |
| `--retry-delay` | `1s` | First retry delay, doubled each retry |
| `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `--log-format` | `text` | `text` or `json`, to stderr |
| `--version` | `false` | Print the version |
| `--circuit-threshold`, `--circuit-timeout` | | Deprecated and ignored |

### Config file

The config keys match the flags in snake_case:

| Area | Keys |
|------|------|
| Targets | `urls`, `file`, `groups` (`<name>: {urls: [...]}`), `group_name` |
| Target defaults | `port`, `protocol`, `timeout` |
| One-shot | `json_output`, `warning_threshold`, `critical_threshold`, `retry_count`, `retry_delay` |
| Modes | `metrics`, `exporter`, `metrics_port` |
| Exporter | `check_interval`, `workers`, `ping_interval`, `reload_interval`, `resolve_interval` |
| Discovery | `discover_nodes`, `discover_exclude_self`, `discovery_interval` |
| Logging | `log_level`, `log_format` |

The file is layered on the defaults, so it only needs the keys it changes. The `circuit_breaker_*` keys and per-group overrides are deprecated: they still parse, log a warning and are ignored.

### Endpoints

| Path | Answer |
|------|--------|
| `/metrics` | Prometheus and OpenMetrics |
| `/healthz` | 200 while the process runs |
| `/readyz` | 200 once the targets are loaded and the checks run; 503 before |
| `/` | Links to the above |

### Metrics

TCP, UDP, HTTP and DNS checks use the labels `url` and `protocol`. ICMP checks use them too, in one-shot and metrics modes. For HTTP, `url` is the full URL; for DNS it is `dns://resolver/name`.

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_current_status` | gauge | `1` if the last check passed, else `0` |
| `urlchecker_total_checks` | counter | Checks run |
| `urlchecker_failed_checks` | counter | Checks failed |
| `urlchecker_response_time_seconds` | histogram | Check time |
| `urlchecker_check_duration_seconds` | histogram | Check time including retries |
| `urlchecker_retry_attempts_total` | counter | Retries (one-shot and metrics modes) |

HTTP and DNS add these, with the label `url`:

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_http_status_code` | gauge | Status of the last response, `0` when there was none |
| `urlchecker_http_version` | gauge | `1.1`, `2`... |
| `urlchecker_http_redirects` | gauge | Redirects followed |
| `urlchecker_http_phase_duration_seconds` | gauge | Last request, label `phase`: `dns`, `connect`, `tls`, `processing` (to the first byte), `transfer` |
| `urlchecker_tls_cert_expiry_timestamp_seconds` | gauge | When the first certificate in the chain expires (Unix time) |
| `urlchecker_dns_answers` | gauge | Answers of the last lookup |

ICMP in exporter mode uses the labels `host` (as written in the target) and `ip` (the resolved address):

| Metric | Type | smokeping_prober equivalent |
|--------|------|-----------------------------|
| `urlchecker_icmp_requests_total` | counter | `smokeping_requests_total` |
| `urlchecker_icmp_response_duration_seconds` | histogram, buckets 50µs × 2ⁿ up to ~26s | `smokeping_response_duration_seconds` |

Group metrics use the label `group`:

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_group_health` | gauge | `1` if every target in the group passed |
| `urlchecker_group_total_urls` | gauge | Targets in the group |
| `urlchecker_group_healthy_urls` | gauge | Targets that passed |

About the exporter itself:

| Metric | Type | Meaning |
|--------|------|---------|
| `urlchecker_target_info` | gauge | `1` per target, with `protocol`, `target` and the target's labels |
| `urlchecker_build_info` | gauge | `1`, with `version`, `revision` and `goversion` |
| `urlchecker_config_last_reload_successful` | gauge | `1` if the last load of the files worked |
| `urlchecker_config_last_reload_success_timestamp_seconds` | gauge | When the files last loaded |
| `urlchecker_discovery_targets` | gauge | Targets from node discovery |
| `urlchecker_discovery_errors_total` | counter | Failed node lists |

## Explanation

### How a check works

- **TCP.** A TCP check opens a connection and closes it right away. It passes when the handshake completes within the timeout, so it proves something is listening, not what the service returns.
- **ICMP.** An ICMP check sends one echo request and waits for the reply that matches its ID and sequence number. It proves the host's network stack answers.
- **HTTP.**
  - An HTTP check makes a full request on a new connection, reads up to 1 MiB of the body, and compares the status and body with the options.
  - The phases come from Go's `httptrace`. Over redirects they add up.
- **DNS.** A DNS check asks one resolver for one record type, over UDP with TCP fallback.

### Why ICMP has its own loop

Packet loss of a few percent is invisible to a check that runs every 30 seconds. One lost packet out of two is a coin toss, and a TCP retransmit hides most of it anyway. To see 1% loss you need many samples, so the exporter pings each ICMP target every `--ping-interval`, one second by default.

It counts every request and every reply. This is how smokeping_prober works, and the metric layout is the same, so existing loss queries need only the metric names changed. The pinger waits for one reply before sending the next request, and a reply that comes after the timeout counts as lost.

### Why the exporter doesn't retry

A retry turns a lost packet or a refused connection into a pass, so the metrics would hide exactly the flapping you want to see. The exporter records every attempt as it happened, and you smooth over time in PromQL. One-shot and metrics modes do retry, because a script usually wants "is it up?", not a sample.

### Raw and unprivileged ICMP sockets

A raw ICMP socket needs `CAP_NET_RAW`, and it receives a copy of every ICMP packet that reaches the host. Each pinger has its own socket and filters by ID, sequence number and source address.

The unprivileged ping socket (Linux `SOCK_DGRAM`) needs no capability, but `net.ipv4.ping_group_range` must cover the process group. On Linux the kernel sets the echo ID and delivers only the matching replies.

urlchecker tries the raw socket first, so it works in both setups without a flag. `df=true` sets `IP_MTU_DISCOVER` (or `IPV6_MTU_DISCOVER`) to "do" on the socket, so the kernel marks every echo don't-fragment.

### What a UDP check proves

UDP has no handshake, and urlchecker sends no payload. A UDP check passes as soon as the name resolves and a route exists, even if nothing listens on the port. Use it to catch DNS and routing failures only; use a `dns://` check for DNS servers, and ICMP or TCP for reachability.

### How reload works

The exporter hashes the content of its config and target files and compares the hash every `--reload-interval`.

It hashes content rather than watching for file events because of how Kubernetes updates a ConfigMap volume. The kubelet swaps a symlink to a new directory, and file-event watchers often miss that, while a content hash catches it. A changed set is diffed against the running one, so unchanged targets keep their state and their pingers.

### Why discovery polls the API with the standard library

Node discovery lists `/api/v1/nodes` every `--discovery-interval`, using the pod's service-account token and CA. It re-reads the token every time, because projected tokens rotate.

A client-go informer would react faster. It would also add tens of megabytes to the binary and a dependency tree for one list call. Nodes come and go in minutes, not seconds, so a 30-second poll is enough.

### Limits

- **Duplicate targets:** a target can be listed only once, because two copies would write the same series. To ping one host with two payload sizes, use its name and its IP.
- **Sockets:** there is one socket per ICMP target. That's fine for tens of targets; a shared socket would scale better to hundreds.
- **`df=true`:** it is Linux only.
- **Discovery:**
  - It checks one InternalIP per node: IPv4 on dual-stack nodes.
  - Discovery settings need a restart; reload covers only the files.

## License

[MIT](LICENSE)
