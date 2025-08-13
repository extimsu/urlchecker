package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TotalChecksCounter tracks the total number of URL checks performed
	TotalChecksCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "urlchecker_total_checks",
			Help: "Total number of URL health checks performed",
		},
		[]string{"url", "protocol"},
	)

	// FailedChecksCounter tracks the number of failed URL checks
	FailedChecksCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "urlchecker_failed_checks",
			Help: "Total number of failed URL health checks",
		},
		[]string{"url", "protocol"},
	)

	// ResponseTimeHistogram tracks the response time distribution
	ResponseTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "urlchecker_response_time_seconds",
			Help:    "Response time in seconds for URL health checks",
			Buckets: prometheus.DefBuckets, // Default buckets: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"url", "protocol"},
	)

	// CurrentStatusGauge tracks the current status of each URL (1 = up, 0 = down)
	CurrentStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_current_status",
			Help: "Current status of URL health checks (1 = up, 0 = down)",
		},
		[]string{"url", "protocol"},
	)

	// CheckDurationHistogram tracks the total time spent on each check
	CheckDurationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "urlchecker_check_duration_seconds",
			Help:    "Total time spent on URL health checks in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"url", "protocol"},
	)
)

// RecordCheck records metrics for a URL health check
func RecordCheck(url, protocol string, success bool, responseTime float64) {
	// Increment total checks counter
	TotalChecksCounter.WithLabelValues(url, protocol).Inc()

	// Increment failed checks counter if check failed
	if !success {
		FailedChecksCounter.WithLabelValues(url, protocol).Inc()
	}

	// Record response time
	ResponseTimeHistogram.WithLabelValues(url, protocol).Observe(responseTime)

	// Update current status gauge
	status := 0.0
	if success {
		status = 1.0
	}
	CurrentStatusGauge.WithLabelValues(url, protocol).Set(status)
}

// RecordCheckDuration records the total duration of a check
func RecordCheckDuration(url, protocol string, duration float64) {
	CheckDurationHistogram.WithLabelValues(url, protocol).Observe(duration)
}

// ResetMetrics resets all metrics (useful for testing)
func ResetMetrics() {
	TotalChecksCounter.Reset()
	FailedChecksCounter.Reset()
	ResponseTimeHistogram.Reset()
	CurrentStatusGauge.Reset()
	CheckDurationHistogram.Reset()
}
