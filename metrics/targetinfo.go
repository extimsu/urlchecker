package metrics

import (
	"slices"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// TargetInfo is one target for urlchecker_target_info.
type TargetInfo struct {
	Protocol string
	Target   string // the url label of the check metrics, or the ICMP host label
	Labels   map[string]string
}

// targetInfo exports urlchecker_target_info{protocol, target, <labels>} 1 for
// the current target set. Each target can bring its own label names, so the
// collector is unchecked: it builds a Desc per target at scrape time.
type targetInfo struct {
	mu      sync.RWMutex
	targets []TargetInfo
}

var info = &targetInfo{}

func init() { prometheus.MustRegister(info) }

// SetTargetInfo replaces the targets reported on urlchecker_target_info.
func SetTargetInfo(targets []TargetInfo) {
	info.mu.Lock()
	info.targets = slices.Clone(targets)
	info.mu.Unlock()
}

// Describe sends nothing: an unchecked collector.
func (*targetInfo) Describe(chan<- *prometheus.Desc) {}

// Collect sends one gauge per target.
func (c *targetInfo) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, t := range c.targets {
		names := []string{"protocol", "target"}
		values := []string{t.Protocol, t.Target}
		keys := make([]string, 0, len(t.Labels))
		for k := range t.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			names = append(names, k)
			values = append(values, t.Labels[k])
		}
		desc := prometheus.NewDesc("urlchecker_target_info",
			"Target labels from the target list, always 1. Join on target (= url or host) to add them to other series.",
			names, nil)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, 1, values...)
	}
}
