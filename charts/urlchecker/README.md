# urlchecker

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.3.0](https://img.shields.io/badge/AppVersion-0.3.0-informational?style=flat-square)

Prometheus exporter for TCP, UDP, ICMP, HTTP(S) and DNS checks. Runs as a Deployment for endpoint checks or as a host-network DaemonSet that finds every node and measures node-to-node packet loss.

The exporter checks TCP, UDP, HTTP(S) and DNS targets every `checks.interval`.
It pings each `icmp://` target every `checks.pingInterval` and exports
smokeping_prober-style counters, so you can alert on packet loss. In a
DaemonSet it can find the nodes itself and measure loss between every pair.
The [urlchecker README](https://github.com/extimsu/urlchecker#readme) covers the
target syntax, every option and every metric.

## Install

```console
helm install probe oci://ghcr.io/extimsu/charts/urlchecker \
  --version 0.3.0 \
  --namespace monitoring --create-namespace \
  --set 'targets={https://example.com/,icmp://1.1.1.1}'
```

Images are on `ghcr.io/extimsu/urlchecker` (also `docker.io/extim/urlchecker`),
multi-arch and signed with cosign; so is the chart.

Upgrade with `helm upgrade`. Remove the release with `helm uninstall probe -n monitoring`.

## Endpoint checks (Deployment)

This is the default. It runs one pod on the pod network that checks each target
from inside the cluster.

```yaml
targets:
  - https://api.example.com/health status=200 body="ok" {team="api"}
  - dns://10.0.0.53/example.com type=A
  - udp://10.0.0.53:53
  - icmp://10.0.0.1 size=1472 df=true
serviceMonitor:
  enabled: true
  labels:
    release: kube-prometheus-stack   # whatever your Prometheus selects
```

The pod runs as the image's non-root user (UID 65532) with every capability
dropped and a read-only root filesystem. For ICMP, the chart sets the safe
sysctl `net.ipv4.ping_group_range` to the pod's own group. That lets the
exporter use the kernel's unprivileged ping socket without `NET_RAW`, and the
pod still passes the `restricted` Pod Security Standard.

## Node-to-node packet loss (DaemonSet)

This mode runs one pod per node in the node's network namespace. With node
discovery on, each pod lists the nodes through the Kubernetes API and pings
every other node's InternalIP, so the loss you measure is on the node network
itself, not on the CNI overlay. New nodes are picked up within
`discovery.nodes.interval`, and removed nodes are dropped with their series.

```yaml
workload:
  kind: DaemonSet
hostNetwork: true
icmp:
  rawSocket: true          # UID 0 with only NET_RAW
tolerations:
  - operator: Exists       # include control-plane nodes
metrics:
  port: 9375               # must be free on every node
service:
  port: 9375
discovery:
  nodes:
    enabled: true          # creates a ClusterRole with list on nodes
    icmp: true
    tcpPorts: [10250]      # also check the kubelet port
    excludeSelf: true      # don't ping your own node
serviceMonitor:
  enabled: true
  relabelings:             # label each series with the probing node
    - sourceLabels: [__meta_kubernetes_pod_node_name]
      targetLabel: node
```

Each discovered target has the label `target_node` on
`urlchecker_target_info`, so you can name the probed node in queries:

```promql
(
  1 - rate(urlchecker_icmp_response_duration_seconds_count[5m])
      / rate(urlchecker_icmp_requests_total[5m])
)
* on (instance, host) group_left (target_node)
  label_replace(urlchecker_target_info{protocol="icmp"}, "host", "$1", "target", "(.*)")
```

To alert per bad node rather than per pair, count how many of a node's peers
see loss, and fire when it is half or more. A single bad path then doesn't
page, and a bad node does.

Pod sysctls can't change the node's own network namespace. So with
`hostNetwork` the unprivileged ping socket works only if the nodes'
`net.ipv4.ping_group_range` already includes the pod's group. If it doesn't,
use `icmp.rawSocket`. The chart refuses ICMP with `hostNetwork` unless
`icmp.rawSocket` is set. A `restricted` namespace rejects this mode, so
install it in a namespace labelled `pod-security.kubernetes.io/enforce: privileged`
or `baseline`.

## Alert rules

```yaml
prometheusRule:
  enabled: true
  labels:
    release: kube-prometheus-stack   # whatever your Prometheus selects
  rules:
    packetLoss:
      threshold: 0.02                # 2%
```

This creates a PrometheusRule with `UrlcheckerTargetDown`,
`UrlcheckerPacketLoss`, `UrlcheckerTLSCertExpiringSoon` and
`UrlcheckerConfigReloadFailed`. Each rule has `enabled`, `for` and `severity`.
`additionalLabels` go on every alert, and `extraRules` are appended. The rules
select this release's series with `namespace="<release namespace>"`, which a
ServiceMonitor scrape sets; set `prometheusRule.selector` for other scrape
setups, e.g. `job="urlchecker"`.

## Targets from your own ConfigMap

```yaml
existingConfigMap: my-targets   # key urls.txt, one target per line
```

urlchecker reloads the target file when its content changes, so a ConfigMap
edit takes effect without a restart. The kubelet syncs ConfigMap volumes about
once a minute, and urlchecker checks every `checks.reloadInterval`.

## Image

`image.tag` defaults to the chart's `appVersion`. For an immutable pin, set
`image.digest`. To pull through a proxy, set `image.registry`, for example
`harbor.example.com/ghcr.io`. Verify the image signature with:

```console
cosign verify ghcr.io/extimsu/urlchecker:0.3.0 \
  --certificate-identity-regexp 'https://github.com/extimsu/urlchecker/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Test

```console
helm test probe -n monitoring
```

The test pod fetches `/metrics` and checks that at least one check has run.

## Requirements

Kubernetes: `>=1.25.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules. |
| automountServiceAccountToken | bool | `false` | Mount the ServiceAccount token. Only node discovery calls the Kubernetes API, and the chart mounts the token when it is on, whatever this says. |
| checks.defaultPort | string | `"80"` | Port for targets without one. |
| checks.defaultProtocol | string | `"tcp"` | Protocol for targets without a prefix: `tcp`, `udp` or `icmp`. |
| checks.interval | string | `"30s"` | Interval between TCP/UDP checks. |
| checks.pingInterval | string | `"1s"` | Interval between echo requests per `icmp://` target. |
| checks.reloadInterval | string | `"30s"` | How often urlchecker checks the target file for changes. |
| checks.timeout | string | `"5s"` | TCP connect timeout. The echo-reply wait is capped at `pingInterval`. |
| checks.workers | int | `5` | Concurrent TCP, UDP, HTTP and DNS checks. |
| commonLabels | object | `{}` | Labels added to every resource. |
| configMapKey | string | `"urls.txt"` | Key of the target list in the ConfigMap. |
| discovery.nodes.enabled | bool | `false` | Check every node in the cluster, found through the Kubernetes API (needs list on nodes; the chart creates the ClusterRole). Targets get the label `target_node` on `urlchecker_target_info`. |
| discovery.nodes.excludeSelf | bool | `true` | Skip the node the pod runs on (useful with a DaemonSet). |
| discovery.nodes.icmp | bool | `true` | Ping each node's InternalIP. |
| discovery.nodes.interval | string | `"30s"` | How often to list the nodes. |
| discovery.nodes.tcpPorts | list | `[]` | Also connect to these TCP ports on each node, e.g. `[10250]` (kubelet). |
| dnsPolicy | string | `""` | Pod DNS policy. Empty means `ClusterFirstWithHostNet` with `hostNetwork`, else the Kubernetes default. |
| existingConfigMap | string | `""` | Use an existing ConfigMap with the target list instead of `targets`. |
| extraArgs | list | `[]` | Extra command-line flags, e.g. `["--warn-threshold=200ms"]`. |
| extraEnv | list | `[]` | Extra environment variables for the container. |
| extraVolumeMounts | list | `[]` | Extra volume mounts for the container. |
| extraVolumes | list | `[]` | Extra volumes for the pod. |
| fullnameOverride | string | `""` | Override the full resource name. |
| grafanaDashboard.annotations | object | `{}` | Annotations, e.g. `grafana_folder: Monitoring` with the sidecar's `folderAnnotation` set. |
| grafanaDashboard.enabled | bool | `false` | Ship the Grafana dashboard (`dashboards/urlchecker.json`) as a ConfigMap for the Grafana sidecar (grafana and kube-prometheus-stack charts) to load. |
| grafanaDashboard.labels | object | `{"grafana_dashboard":"1"}` | Labels the sidecar selects on. The grafana chart's default is `grafana_dashboard: "1"`. |
| grafanaDashboard.namespace | string | `""` | Namespace for the ConfigMap. Defaults to the release namespace; set it if the sidecar watches only its own namespace. |
| hostNetwork | bool | `false` | Use the node's network namespace. Set it for node-to-node probes with `workload.kind: DaemonSet`. Choose a free `metrics.port` on the nodes. |
| icmp.rawSocket | bool | `false` | Open raw ICMP sockets: runs the container as UID 0 with only the `NET_RAW` capability. Needed with `hostNetwork: true` when the nodes' `net.ipv4.ping_group_range` does not include the pod's group. When false, the pod gets the `net.ipv4.ping_group_range` sysctl so the non-root user can use the unprivileged ping socket (pod network only). |
| image.digest | string | `""` | Image digest (`sha256:...`). When set it wins over `tag`. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| image.registry | string | `"ghcr.io"` | Image registry. Images are also on `docker.io/extim/urlchecker`. |
| image.repository | string | `"extimsu/urlchecker"` | Image repository. |
| image.tag | string | `""` | Image tag. Defaults to the chart appVersion. |
| imagePullSecrets | list | `[]` | Pull secrets, e.g. `[{name: regcred}]`. |
| livenessProbe | object | `{"failureThreshold":3,"httpGet":{"path":"/healthz","port":"metrics"},"periodSeconds":30}` | Liveness probe: the process is alive. |
| metrics.port | int | `9090` | Port of the `/metrics` endpoint. |
| nameOverride | string | `""` | Override the chart name used in resource names. |
| nodeSelector | object | `{}` | Node selector. |
| podAnnotations | object | `{}` | Pod annotations. |
| podDisruptionBudget.enabled | bool | `false` | Create a PodDisruptionBudget (Deployment only). |
| podDisruptionBudget.minAvailable | int | `1` | Minimum available pods. |
| podLabels | object | `{}` | Pod labels. |
| podSecurityContext | object | `{"fsGroup":65532,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod security context. The UID/GID match the image's distroless `nonroot` user; `icmp.rawSocket` switches the container to UID 0. |
| priorityClassName | string | `""` | Priority class name. |
| prometheusRule.additionalLabels | object | `{}` | Labels added to every alert. |
| prometheusRule.enabled | bool | `false` | Create a Prometheus Operator PrometheusRule with the alerts below. |
| prometheusRule.extraRules | list | `[]` | Extra Prometheus rules appended to the group. |
| prometheusRule.labels | object | `{}` | Extra labels, e.g. the selector your Prometheus uses for rules. |
| prometheusRule.namespace | string | `""` | Namespace for the PrometheusRule. Defaults to the release namespace. |
| prometheusRule.rules.certExpiry.days | int | `14` |  |
| prometheusRule.rules.certExpiry.enabled | bool | `true` | Alert when an HTTPS certificate expires within `days`. |
| prometheusRule.rules.certExpiry.for | string | `"1h"` |  |
| prometheusRule.rules.certExpiry.severity | string | `"warning"` |  |
| prometheusRule.rules.packetLoss.enabled | bool | `true` | Alert on ICMP loss above `threshold` (0.05 = 5%) over 5 minutes. |
| prometheusRule.rules.packetLoss.for | string | `"10m"` |  |
| prometheusRule.rules.packetLoss.severity | string | `"warning"` |  |
| prometheusRule.rules.packetLoss.threshold | float | `0.05` |  |
| prometheusRule.rules.reloadFailed.enabled | bool | `true` | Alert when the target file cannot be loaded. |
| prometheusRule.rules.reloadFailed.for | string | `"10m"` |  |
| prometheusRule.rules.reloadFailed.severity | string | `"warning"` |  |
| prometheusRule.rules.targetDown.enabled | bool | `true` | Alert when a TCP, UDP, HTTP or DNS check keeps failing. |
| prometheusRule.rules.targetDown.for | string | `"5m"` |  |
| prometheusRule.rules.targetDown.severity | string | `"warning"` |  |
| prometheusRule.selector | string | `""` | PromQL label matchers that pick this release's series. Defaults to `namespace="<release namespace>"`, which a ServiceMonitor scrape sets. |
| rbac.create | bool | `true` | Create the ClusterRole and ClusterRoleBinding node discovery needs. |
| readinessProbe | object | `{"httpGet":{"path":"/readyz","port":"metrics"},"periodSeconds":10}` | Readiness probe: targets loaded and checks running. |
| replicaCount | int | `1` | Pods for `workload.kind: Deployment`. Each replica checks every target, so more than one replica only adds vantage points. |
| resources | object | `{"limits":{"memory":"64Mi"},"requests":{"cpu":"10m","memory":"32Mi"}}` | Resource requests and limits. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container security context. `icmp.rawSocket` adds `NET_RAW`. |
| service.annotations | object | `{}` | Service annotations. |
| service.enabled | bool | `true` | Create a Service for the metrics port. |
| service.port | int | `9090` | Service port. |
| service.type | string | `"ClusterIP"` | Service type. |
| serviceAccount.annotations | object | `{}` | ServiceAccount annotations. |
| serviceAccount.create | bool | `true` | Create a ServiceAccount. |
| serviceAccount.name | string | `""` | ServiceAccount name. Generated when empty and `create` is true. |
| serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor (the VictoriaMetrics operator converts it too). Needs `service.enabled`. |
| serviceMonitor.interval | string | `"30s"` | Scrape interval. |
| serviceMonitor.labels | object | `{}` | Extra labels, e.g. the selector your Prometheus uses. |
| serviceMonitor.metricRelabelings | list | `[]` | Relabelings applied to scraped samples. |
| serviceMonitor.namespace | string | `""` | Namespace for the ServiceMonitor. Defaults to the release namespace. |
| serviceMonitor.relabelings | list | `[]` | Relabelings applied before the scrape. |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout. |
| targets | list | `[]` | Targets to check, one per entry: `host:port`, `tcp://`, `udp://`, `icmp://`, `http(s)://` URLs or `dns://[resolver]/name`, each with optional `key=value` options and a `{label="value"}` block. Rendered into a ConfigMap that urlchecker reloads on change. Ignored when `existingConfigMap` is set. |
| terminationGracePeriodSeconds | int | `10` | Termination grace period. |
| tests.image | string | `"docker.io/library/busybox:1.37.0"` | Image for the `helm test` pod that fetches `/metrics`. |
| tolerations | list | `[]` | Tolerations. For a DaemonSet on every node, use `[{operator: Exists}]`. |
| topologySpreadConstraints | list | `[]` | Topology spread constraints. |
| updateStrategy | object | `{"type":"RollingUpdate"}` | Deployment strategy or DaemonSet updateStrategy. |
| workload.kind | string | `"Deployment"` | `Deployment` checks endpoints from the pod network. `DaemonSet` runs one pod per node, usually with `hostNetwork: true`, for node-to-node checks. |

## Development

```console
helm lint charts/urlchecker --strict --set 'targets={a:1}'
helm unittest charts/urlchecker
helm-docs --chart-search-root charts   # regenerate this README
```

CI runs the same checks plus install tests on kind: a Deployment in a
`restricted` namespace, a host-network DaemonSet, and node discovery on a
two-node cluster, all with an image built from the same commit. Publishing
happens on a `chart-v<version>` tag. The dashboard JSON is generated by
`hack/gen-dashboard.py`; CI fails when they differ.
