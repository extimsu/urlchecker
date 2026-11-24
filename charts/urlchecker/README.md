# urlchecker

![Version: 0.2.0](https://img.shields.io/badge/Version-0.2.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.2.0](https://img.shields.io/badge/AppVersion-0.2.0-informational?style=flat-square)

Prometheus exporter that checks TCP, UDP and ICMP reachability. Runs as a Deployment for endpoint checks or as a host-network DaemonSet for node-to-node packet loss.

The exporter checks TCP and UDP targets every `checks.interval`. It pings each
`icmp://` target every `checks.pingInterval` and exports
smokeping_prober-style counters, so you can alert on packet loss. The
[urlchecker README](https://github.com/extimsu/urlchecker#readme) covers the
target syntax and every metric.

## Install

```console
helm install probe oci://ghcr.io/extimsu/charts/urlchecker \
  --version 0.2.0 \
  --namespace monitoring --create-namespace \
  --set 'targets={example.com:443,icmp://1.1.1.1}'
```

Upgrade with `helm upgrade`. Remove the release with `helm uninstall probe -n monitoring`.

## Endpoint checks (Deployment)

This is the default. It runs one pod on the pod network that checks each target
from inside the cluster.

```yaml
targets:
  - api.example.com:443
  - udp://10.0.0.53:53
  - icmp://10.0.0.1
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

This mode runs one pod per node in the node's network namespace. Each pod pings
every node, so the loss you measure is on the node network itself, not on the
CNI overlay.

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
targets:                   # every node's internal IP, plus a TCP port to check
  - icmp://10.0.0.11
  - 10.0.0.11:10250
  - icmp://10.0.0.12
  - 10.0.0.12:10250
serviceMonitor:
  enabled: true
  relabelings:             # label each series with the probing node
    - sourceLabels: [__meta_kubernetes_pod_node_name]
      targetLabel: node
```

Pod sysctls can't change the node's own network namespace. So with
`hostNetwork` the unprivileged ping socket works only if the nodes'
`net.ipv4.ping_group_range` already includes the pod's group. If it doesn't,
use `icmp.rawSocket`. The chart refuses `icmp://` targets with `hostNetwork`
unless `icmp.rawSocket` is set. A `restricted` namespace rejects this mode, so
install it in a namespace labelled `pod-security.kubernetes.io/enforce: privileged`
or `baseline`.

The loss per pair, from the probing node to each target, is:

```promql
1 - (
  rate(urlchecker_icmp_response_duration_seconds_count[5m])
    / rate(urlchecker_icmp_requests_total[5m])
)
```

To alert per bad node rather than per pair, count how many of a node's peers
see loss, and fire when it is half or more. A single bad path then doesn't
page, and a bad node does.

## Grafana dashboard

```yaml
grafanaDashboard:
  enabled: true
  annotations:
    grafana_folder: Monitoring   # with the sidecar's folderAnnotation set
```

This ships [`dashboards/urlchecker.json`](dashboards/urlchecker.json) as a
ConfigMap labelled `grafana_dashboard: "1"`. The Grafana sidecar of the grafana
and kube-prometheus-stack charts loads ConfigMaps with that label. If the
sidecar watches only its own namespace, set `grafanaDashboard.namespace` to that
namespace. Without the sidecar, import the JSON file in Grafana under
**Dashboards → New → Import**.

The dashboard shows:

- target status and p95 response time;
- ICMP loss and round trip;
- a smokeping-style round-trip heatmap;
- a loss matrix of instance × target.

In DaemonSet mode, each instance is one node.

## Targets from your own ConfigMap

```yaml
existingConfigMap: my-targets   # key urls.txt, one target per line
```

When the ConfigMap changes, restart the pods yourself, because the chart only
hashes `targets`.

## Image

`image.tag` defaults to the chart's `appVersion`. For an immutable pin, set
`image.digest`. To pull through a proxy, set `image.registry`, for example
`harbor.example.com/docker.io`.

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
| automountServiceAccountToken | bool | `false` | Mount the ServiceAccount token. urlchecker does not call the Kubernetes API. |
| checks.defaultPort | string | `"80"` | Port for targets without one. |
| checks.defaultProtocol | string | `"tcp"` | Protocol for targets without a prefix: `tcp`, `udp` or `icmp`. |
| checks.interval | string | `"30s"` | Interval between TCP/UDP checks. |
| checks.pingInterval | string | `"1s"` | Interval between echo requests per `icmp://` target. |
| checks.timeout | string | `"5s"` | TCP connect timeout. The echo-reply wait is capped at `pingInterval`. |
| checks.workers | int | `5` | Concurrent TCP/UDP checks. |
| commonLabels | object | `{}` | Labels added to every resource. |
| configMapKey | string | `"urls.txt"` | Key of the target list in the ConfigMap. |
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
| image.registry | string | `"docker.io"` | Image registry. |
| image.repository | string | `"extim/urlchecker"` | Image repository. |
| image.tag | string | `""` | Image tag. Defaults to the chart appVersion. |
| imagePullSecrets | list | `[]` | Pull secrets, e.g. `[{name: regcred}]`. |
| livenessProbe.failureThreshold | int | `3` |  |
| livenessProbe.httpGet.path | string | `"/metrics"` |  |
| livenessProbe.httpGet.port | string | `"metrics"` |  |
| livenessProbe.periodSeconds | int | `30` |  |
| metrics.port | int | `9090` | Port of the `/metrics` endpoint. |
| nameOverride | string | `""` | Override the chart name used in resource names. |
| nodeSelector | object | `{}` | Node selector. |
| podAnnotations | object | `{}` | Pod annotations. |
| podDisruptionBudget.enabled | bool | `false` | Create a PodDisruptionBudget (Deployment only). |
| podDisruptionBudget.minAvailable | int | `1` | Minimum available pods. |
| podLabels | object | `{}` | Pod labels. |
| podSecurityContext | object | `{"fsGroup":65532,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod security context. The UID/GID match the image's distroless `nonroot` user; `icmp.rawSocket` switches the container to UID 0. |
| priorityClassName | string | `""` | Priority class name. |
| readinessProbe.httpGet.path | string | `"/metrics"` |  |
| readinessProbe.httpGet.port | string | `"metrics"` |  |
| readinessProbe.periodSeconds | int | `10` |  |
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
| targets | list | `[]` | Targets to check, one per entry. `host`, `host:port`, `tcp://host:port`, `udp://host:port` or `icmp://host`. Rendered into a ConfigMap. Ignored when `existingConfigMap` is set. |
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

CI runs the same checks plus an install test on kind in both modes. Publishing
happens on a `chart-v<version>` tag.
