# Changelog

All notable changes to urlchecker. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[semantic versioning](https://semver.org/). The Helm chart has its own version;
its changes are in `charts/urlchecker/Chart.yaml` (`artifacthub.io/changes`).

## [0.3.0] - 2026-11-24

### Added

- HTTP and HTTPS checks. The options are `method`, `status`, `body`,
  `follow`, `insecure` and `host`. The new metrics are:
  - `urlchecker_http_status_code`, `urlchecker_http_version` and `urlchecker_http_redirects`;
  - `urlchecker_http_phase_duration_seconds` (DNS, connect, TLS, processing, transfer);
  - `urlchecker_tls_cert_expiry_timestamp_seconds`, which is reported even for an expired or untrusted certificate.
- DNS checks: `dns://[resolver[:port]]/name`, with `type` and `expect`, and
  `urlchecker_dns_answers`.
- Target options (`timeout`, `ip`, and `size` and `df` for ICMP) and a
  `{label="value"}` block per target, exported on `urlchecker_target_info`.
- IPv6 for every protocol, ICMP included. `ip=4` or `ip=6` picks the family.
- MTU checks: `size=` and `df=true` (don't-fragment, Linux).
- Hot reload in exporter mode, on SIGHUP or a file change every
  `--reload-interval`. The new metrics are `urlchecker_config_last_reload_successful`
  and `urlchecker_config_last_reload_success_timestamp_seconds`.
- ICMP host names are resolved again every `--resolve-interval`.
- Kubernetes node discovery: `--discover-nodes icmp,tcp:<port>`,
  `--discover-exclude-self` and `--discovery-interval`. Targets carry `target_node`.
- `/healthz`, `/readyz` and an index page next to `/metrics`, and
  `urlchecker_build_info`.
- Group metrics in exporter mode.
- `--log-level` and `--log-format` (text or json) with structured logs. The
  exporter logs only state changes.
- Signed multi-arch images on `ghcr.io/extimsu/urlchecker`, and binaries for
  Linux, macOS and Windows with checksums, SBOMs and cosign signatures, built
  by CI on every `v*` tag.
- Helm chart 0.3.0:
  - `discovery.nodes` with its RBAC;
  - a PrometheusRule (`prometheusRule.enabled`);
  - `/healthz` and `/readyz` probes;
  - HTTP, DNS and exporter panels on the dashboard;
  - install tests that run against an image built from the same commit.

### Changed

- **Breaking:** one-shot mode exits 1 when a target fails, and 2 on a usage
  or config error. It used to exit 0.
- **Breaking:** the image moved to `ghcr.io/extimsu/urlchecker`. It is built
  on `distroless/static-debian12:nonroot`, and the binary is now
  `/usr/local/bin/urlchecker`.
- **Breaking:** the chart no longer restarts pods when the targets change,
  because urlchecker reloads them.
- One-shot results print in the order of the targets.
- A target listed twice is an error.
- Go 1.26 or later is needed to build.

### Removed

- **Breaking:** the circuit breaker. It never ran in any mode. The
  `urlchecker_circuit_breaker_*` metrics are gone. `--circuit-threshold`,
  `--circuit-timeout`, the `circuit_breaker_*` keys and the per-group override
  keys still parse, log a deprecation warning and are ignored.

### Fixed

- One-shot and metrics modes could report a result for the wrong target: every
  check wrote to one shared struct.
- Flag defaults overrode the config file (for example `port:`). Only flags
  given on the command line override it now.
- A config file without `metrics_port` or `workers` failed validation.
- `--metrics` mode never started its metrics server.
- The exporter could queue a second check of a target whose last check was
  still running.

## [0.2.0] - 2026-11-23

### Added

- `icmp://` targets with smokeping_prober-style metrics
  (`urlchecker_icmp_requests_total`, `urlchecker_icmp_response_duration_seconds`).
- `--ping-interval`.
- The official Helm chart, published to `oci://ghcr.io/extimsu/charts/urlchecker`.

[0.3.0]: https://github.com/extimsu/urlchecker/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/extimsu/urlchecker/releases/tag/v0.2.0
