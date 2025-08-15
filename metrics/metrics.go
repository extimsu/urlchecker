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

	// GroupHealthGauge tracks the health status of groups (1 = healthy, 0 = unhealthy)
	GroupHealthGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_group_health",
			Help: "Health status of URL groups (1 = healthy, 0 = unhealthy)",
		},
		[]string{"group"},
	)

	// GroupTotalURLsGauge tracks the total number of URLs in each group
	GroupTotalURLsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_group_total_urls",
			Help: "Total number of URLs in each group",
		},
		[]string{"group"},
	)

	// GroupHealthyURLsGauge tracks the number of healthy URLs in each group
	GroupHealthyURLsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_group_healthy_urls",
			Help: "Number of healthy URLs in each group",
		},
		[]string{"group"},
	)

	// RetryAttemptsCounter tracks the total number of retry attempts
	RetryAttemptsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "urlchecker_retry_attempts_total",
			Help: "Total number of retry attempts for URL health checks",
		},
		[]string{"url", "protocol"},
	)

	// CircuitBreakerStateGauge tracks the current state of circuit breakers
	CircuitBreakerStateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_circuit_breaker_state",
			Help: "Current state of circuit breakers (0 = closed, 1 = half-open, 2 = open)",
		},
		[]string{"url", "protocol"},
	)

	// CircuitBreakerTransitionsCounter tracks circuit breaker state transitions
	CircuitBreakerTransitionsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "urlchecker_circuit_breaker_transitions_total",
			Help: "Total number of circuit breaker state transitions",
		},
		[]string{"url", "protocol", "transition"},
	)

	// CircuitBreakerFailureCountGauge tracks the current failure count for each circuit breaker
	CircuitBreakerFailureCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "urlchecker_circuit_breaker_failure_count",
			Help: "Current consecutive failure count for each circuit breaker",
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

// RecordGroupHealth records group-level metrics
func RecordGroupHealth(groupName string, isHealthy bool, totalURLs, healthyURLs int) {
	// Record group health status (1 = healthy, 0 = unhealthy)
	healthStatus := 0.0
	if isHealthy {
		healthStatus = 1.0
	}
	GroupHealthGauge.WithLabelValues(groupName).Set(healthStatus)

	// Record total URLs in group
	GroupTotalURLsGauge.WithLabelValues(groupName).Set(float64(totalURLs))

	// Record healthy URLs in group
	GroupHealthyURLsGauge.WithLabelValues(groupName).Set(float64(healthyURLs))
}

// RecordRetryAttempt records a retry attempt for a URL
func RecordRetryAttempt(url, protocol string) {
	RetryAttemptsCounter.WithLabelValues(url, protocol).Inc()
}

// RecordCircuitBreakerState records the current state of a circuit breaker
func RecordCircuitBreakerState(url, protocol string, state int) {
	CircuitBreakerStateGauge.WithLabelValues(url, protocol).Set(float64(state))
}

// RecordCircuitBreakerTransition records a circuit breaker state transition
func RecordCircuitBreakerTransition(url, protocol, transition string) {
	CircuitBreakerTransitionsCounter.WithLabelValues(url, protocol, transition).Inc()
}

// RecordCircuitBreakerFailureCount records the current failure count for a circuit breaker
func RecordCircuitBreakerFailureCount(url, protocol string, failureCount int) {
	CircuitBreakerFailureCountGauge.WithLabelValues(url, protocol).Set(float64(failureCount))
}

// ResetMetrics resets all metrics (useful for testing)
func ResetMetrics() {
	TotalChecksCounter.Reset()
	FailedChecksCounter.Reset()
	ResponseTimeHistogram.Reset()
	CurrentStatusGauge.Reset()
	CheckDurationHistogram.Reset()
	GroupHealthGauge.Reset()
	GroupTotalURLsGauge.Reset()
	GroupHealthyURLsGauge.Reset()
	RetryAttemptsCounter.Reset()
	CircuitBreakerStateGauge.Reset()
	CircuitBreakerTransitionsCounter.Reset()
	CircuitBreakerFailureCountGauge.Reset()
}
