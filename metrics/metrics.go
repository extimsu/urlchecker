// Package metrics defines the Prometheus metrics urlchecker exports.
package metrics

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var checkLabels = []string{"url", "protocol"}

var (
	// TotalChecksCounter counts TCP and UDP checks (and ICMP in one-shot and metrics modes).
	TotalChecksCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "urlchecker_total_checks",
		Help: "Total number of URL health checks performed",
	}, checkLabels)

	// FailedChecksCounter counts failed checks.
	FailedChecksCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "urlchecker_failed_checks",
		Help: "Total number of failed URL health checks",
	}, checkLabels)

	// ResponseTimeHistogram holds the time of each check.
	ResponseTimeHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "urlchecker_response_time_seconds",
		Help:    "Response time in seconds for URL health checks",
		Buckets: prometheus.DefBuckets,
	}, checkLabels)

	// CurrentStatusGauge is 1 when the last check passed, else 0.
	CurrentStatusGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_current_status",
		Help: "Current status of URL health checks (1 = up, 0 = down)",
	}, checkLabels)

	// CheckDurationHistogram holds the time of each check including retries.
	CheckDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "urlchecker_check_duration_seconds",
		Help:    "Total time spent on URL health checks in seconds",
		Buckets: prometheus.DefBuckets,
	}, checkLabels)

	// GroupHealthGauge is 1 when every target in the group is up.
	GroupHealthGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_group_health",
		Help: "Health status of URL groups (1 = healthy, 0 = unhealthy)",
	}, []string{"group"})

	// GroupTotalURLsGauge is the number of targets in a group.
	GroupTotalURLsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_group_total_urls",
		Help: "Total number of URLs in each group",
	}, []string{"group"})

	// GroupHealthyURLsGauge is the number of targets in a group that are up.
	GroupHealthyURLsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_group_healthy_urls",
		Help: "Number of healthy URLs in each group",
	}, []string{"group"})

	// RetryAttemptsCounter counts retries (one-shot and metrics modes only).
	RetryAttemptsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "urlchecker_retry_attempts_total",
		Help: "Total number of retry attempts for URL health checks",
	}, checkLabels)

	// ICMPRequests counts echo requests sent to icmp:// targets (smokeping_requests_total).
	ICMPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "urlchecker_icmp_requests_total",
		Help: "ICMP echo requests sent",
	}, []string{"host", "ip"})

	// ICMPResponseDuration holds the round-trip time of every echo reply
	// (smokeping_response_duration_seconds). Its _count is the number of replies.
	ICMPResponseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "urlchecker_icmp_response_duration_seconds",
		Help:    "ICMP echo round-trip time in seconds",
		Buckets: prometheus.ExponentialBuckets(0.00005, 2, 20), // 50µs to ~26s, as smokeping_prober
	}, []string{"host", "ip"})

	buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_build_info",
		Help: "Build information, always 1",
	}, []string{"version", "revision", "goversion"})
)

// SetBuildInfo sets urlchecker_build_info.
func SetBuildInfo(version, revision string) {
	buildInfo.WithLabelValues(version, revision, runtime.Version()).Set(1)
}

// RecordCheck records one TCP, UDP or one-shot ICMP check.
func RecordCheck(url, protocol string, success bool, responseTime float64) {
	TotalChecksCounter.WithLabelValues(url, protocol).Inc()
	status := 1.0
	if !success {
		FailedChecksCounter.WithLabelValues(url, protocol).Inc()
		status = 0
	}
	ResponseTimeHistogram.WithLabelValues(url, protocol).Observe(responseTime)
	CurrentStatusGauge.WithLabelValues(url, protocol).Set(status)
}

// RecordCheckDuration records the total duration of a check, including retries.
func RecordCheckDuration(url, protocol string, duration float64) {
	CheckDurationHistogram.WithLabelValues(url, protocol).Observe(duration)
}

// RecordGroupHealth records the health of one group.
func RecordGroupHealth(group string, healthy bool, total, healthyCount int) {
	h := 0.0
	if healthy {
		h = 1
	}
	GroupHealthGauge.WithLabelValues(group).Set(h)
	GroupTotalURLsGauge.WithLabelValues(group).Set(float64(total))
	GroupHealthyURLsGauge.WithLabelValues(group).Set(float64(healthyCount))
}

// DeleteGroup removes the series of a group that no longer has targets.
func DeleteGroup(group string) {
	GroupHealthGauge.DeleteLabelValues(group)
	GroupTotalURLsGauge.DeleteLabelValues(group)
	GroupHealthyURLsGauge.DeleteLabelValues(group)
}

// RecordRetryAttempt counts one retry.
func RecordRetryAttempt(url, protocol string) {
	RetryAttemptsCounter.WithLabelValues(url, protocol).Inc()
}

// DeleteCheck removes every series of a TCP or UDP target, so a target that
// left the list doesn't keep reporting its last state.
func DeleteCheck(url, protocol string) {
	for _, v := range []interface {
		DeleteLabelValues(...string) bool
	}{TotalChecksCounter, FailedChecksCounter, ResponseTimeHistogram, CurrentStatusGauge, CheckDurationHistogram, RetryAttemptsCounter} {
		v.DeleteLabelValues(url, protocol)
	}
}

// DeleteICMP removes every series of an ICMP target, for any resolved IP.
func DeleteICMP(host string) {
	ICMPRequests.DeletePartialMatch(prometheus.Labels{"host": host})
	ICMPResponseDuration.DeletePartialMatch(prometheus.Labels{"host": host})
}

// ResetMetrics resets every vector. Tests use it.
func ResetMetrics() {
	for _, v := range []interface{ Reset() }{
		TotalChecksCounter, FailedChecksCounter, ResponseTimeHistogram, CurrentStatusGauge, CheckDurationHistogram,
		GroupHealthGauge, GroupTotalURLsGauge, GroupHealthyURLsGauge, RetryAttemptsCounter, ICMPRequests, ICMPResponseDuration,
	} {
		v.Reset()
	}
}
