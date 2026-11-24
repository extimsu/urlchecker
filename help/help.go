// Package help prints the short usage.
package help

import (
	"fmt"
	"io"
)

// Show prints the short usage to w.
func Show(w io.Writer) {
	_, _ = fmt.Fprint(w, `urlchecker checks TCP, UDP and ICMP reachability.

Usage:
  urlchecker --url example.com:443[,icmp://10.0.0.1]   check once, exit 1 if a target is down
  urlchecker --file targets.txt                         one target per line
  urlchecker --metrics --check-interval 30s             check repeatedly and serve /metrics
  urlchecker --exporter --file targets.txt              Prometheus exporter; ICMP every --ping-interval
  urlchecker --config urlchecker.yaml                   YAML or JSON config; flags override it

Run urlchecker --help for every flag.
`)
}
