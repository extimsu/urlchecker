# urlchecker

A powerful CLI tool for health-checking URLs with Prometheus metrics support and continuous monitoring capabilities.

## Features

- **Basic URL Health Checks**: Check single or multiple URLs with custom ports and protocols
- **File-based URL Lists**: Import URLs from text files for batch processing
- **JSON Output**: Get structured JSON responses for integration
- **Prometheus Metrics**: Basic metrics mode with continuous monitoring
- **Advanced Exporter Mode**: Production-ready exporter with worker pools and thread-safe state management
- **Docker Support**: Run as a containerized application

## Getting Started

### Prerequisites

This project requires Go to be installed. On OS X with Homebrew you can just run `brew install go`.

### Building

```console
make build
cd ./bin/
```

### Basic Usage

#### Single URL Check

```console
./urlchecker --url extim.su
```

#### Multiple URLs

```console
./urlchecker --url extim.su,google.com:80,example.com:443
```

#### Custom Protocol (TCP/UDP)

```console
./urlchecker --url google.com:53 --protocol udp
```

#### File-based URL Lists with JSON Output

```console
./urlchecker --file url.txt --json
```

### Prometheus Metrics Mode

#### Basic Metrics Mode

Enable continuous monitoring with Prometheus metrics:

```console
./urlchecker --metrics --url google.com,github.com --check-interval 30s
```

This will:

- Start a Prometheus metrics server on port 9090 (default)
- Continuously monitor the specified URLs
- Expose metrics at `http://localhost:9090/metrics`

#### Advanced Exporter Mode (Recommended)

For production environments, use the exporter mode with worker pools:

```console
./urlchecker --exporter --url google.com,github.com --workers 5 --check-interval 30s
```

Exporter mode includes:

- **Worker Pool**: Configurable number of concurrent workers
- **Thread-safe State Management**: Comprehensive URL state tracking
- **Enhanced Logging**: Detailed worker activity and response times
- **Prometheus Metrics**: Automatically included (no separate flag needed)
- **Graceful Shutdown**: Proper signal handling and resource cleanup

#### Exporter Mode Options

```console
# Custom worker count and monitoring interval
./urlchecker --exporter --url google.com,github.com --workers 3 --check-interval 10s

# Custom metrics port
./urlchecker --exporter --url google.com --workers 2 --metrics-port 8080

# Monitor URLs from file
./urlchecker --exporter --file urls.txt --workers 4 --check-interval 60s
```

### Using Docker

#### Basic URL Check

```console
docker run --rm docker.io/extim/urlchecker --url extim.su
```

#### Multiple URLs with Custom Ports

```console
docker run --rm docker.io/extim/urlchecker --url extim.su,google.com:80,example.com:443
```

#### Custom Default Port

```console
docker run --rm docker.io/extim/urlchecker --url extim.su,google.com:80,example.com --port 443
```

#### UDP Protocol with JSON Output

```console
docker run --rm docker.io/extim/urlchecker --url google.com:53 --protocol udp --json
```

#### File-based URL Lists

```console
docker run --rm -v ./urls.txt:/opt/urlchecker/bin/url.txt docker.io/extim/urlchecker --file url.txt
```

#### Exporter Mode with Docker

```console
# Expose metrics port for Prometheus scraping
docker run --rm -p 9090:9090 docker.io/extim/urlchecker --exporter --url google.com,github.com --workers 3

# Custom metrics port
docker run --rm -p 8080:8080 docker.io/extim/urlchecker --exporter --url google.com --metrics-port 8080
```

## Available Metrics

When running in metrics or exporter mode, the following Prometheus metrics are available at `/metrics`:

- `urlchecker_total_checks`: Total number of URL health checks performed
- `urlchecker_failed_checks`: Number of failed URL checks
- `urlchecker_response_time_seconds`: Response time histogram
- `urlchecker_check_duration_seconds`: Check duration histogram
- `urlchecker_current_status`: Current status gauge (1 = up, 0 = down)

All metrics include labels for `url` and `protocol` for detailed monitoring.

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--url` | URL(s) to check (comma-separated) | - |
| `--file` | File containing URLs (one per line) | - |
| `--port` | Default port for URLs | 80 |
| `--protocol` | Protocol to use (tcp/udp) | tcp |
| `--timeout` | Connection timeout | 5s |
| `--json` | Output results in JSON format | false |
| `--metrics` | Enable basic Prometheus metrics mode | false |
| `--exporter` | Enable advanced exporter mode (includes metrics) | false |
| `--workers` | Number of worker goroutines (exporter mode) | 5 |
| `--metrics-port` | Port for Prometheus metrics endpoint | 9090 |
| `--check-interval` | Interval between health checks | 30s |
| `--version` | Show version information | false |

## Examples

### Development and Testing

```console
# Quick single URL check
./urlchecker --url google.com

# Multiple URLs with custom timeout
./urlchecker --url google.com,github.com,example.com --timeout 3s

# JSON output for scripting
./urlchecker --url google.com --json
```

### Production Monitoring

```console
# Basic metrics mode
./urlchecker --metrics --url google.com,github.com --check-interval 60s

# Advanced exporter mode (recommended)
./urlchecker --exporter --url google.com,github.com,example.com --workers 5 --check-interval 30s
```

### Containerized Deployment

```console
# Run exporter mode in Docker with port exposure
docker run -d --name urlchecker \
  -p 9090:9090 \
  docker.io/extim/urlchecker \
  --exporter \
  --url google.com,github.com,example.com \
  --workers 3 \
  --check-interval 30s
```
