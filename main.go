package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/extimsu/urlchecker/config"
	"github.com/extimsu/urlchecker/help"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/extimsu/urlchecker/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Search struct {
	Url              string
	Port             string
	Protocol         string
	Timeout          time.Duration
	WarnThreshold    time.Duration
	CritThreshold    time.Duration
	RetryCount       int
	RetryDelay       time.Duration
	CircuitThreshold int
	CircuitTimeout   time.Duration
	SearchResult
}

type SearchResult struct {
	Address      string  `json:"address"`
	Port         string  `json:"port"`
	State        string  `json:"state"`
	ResponseTime float64 `json:"response_time_seconds"`
	Group        string  `json:"group,omitempty"`
}

// URLWithGroup represents a URL with its associated group
type URLWithGroup struct {
	URL   string
	Group string
}

// GroupStatus represents the health status of a group
type GroupStatus struct {
	GroupName     string   `json:"group_name"`
	IsHealthy     bool     `json:"is_healthy"`
	TotalURLs     int      `json:"total_urls"`
	HealthyURLs   int      `json:"healthy_urls"`
	UnhealthyURLs int      `json:"unhealthy_urls"`
	URLs          []string `json:"urls"`
}

// GroupResult represents a group with its URLs and health status
type GroupResult struct {
	GroupName     string         `json:"group_name"`
	IsHealthy     bool           `json:"is_healthy"`
	TotalURLs     int            `json:"total_urls"`
	HealthyURLs   int            `json:"healthy_urls"`
	UnhealthyURLs int            `json:"unhealthy_urls"`
	URLs          []SearchResult `json:"urls"`
}

// HealthCheckResult represents the complete health check result with groups
type HealthCheckResult struct {
	Groups        []GroupResult  `json:"groups,omitempty"`
	UngroupedURLs []SearchResult `json:"ungrouped_urls,omitempty"`
	Summary       struct {
		TotalGroups     int `json:"total_groups"`
		HealthyGroups   int `json:"healthy_groups"`
		UnhealthyGroups int `json:"unhealthy_groups"`
		TotalURLs       int `json:"total_urls"`
		HealthyURLs     int `json:"healthy_urls"`
		UnhealthyURLs   int `json:"unhealthy_urls"`
	} `json:"summary"`
}

// URLState represents the current state of a URL check
type URLState struct {
	URL          string
	Protocol     string
	LastCheck    time.Time
	LastSuccess  time.Time
	LastFailure  time.Time
	ResponseTime float64
	IsUp         bool
	CheckCount   int64
	FailureCount int64
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitHalfOpen
	CircuitOpen
)

// String returns the string representation of the circuit breaker state
func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitHalfOpen:
		return "half-open"
	case CircuitOpen:
		return "open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	threshold    int
	timeout      time.Duration
	failureCount int
	lastFailure  time.Time
	state        CircuitBreakerState
	mu           sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
		state:     CircuitClosed,
	}
}

// IsOpen checks if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailure) >= cb.timeout {
			cb.state = CircuitHalfOpen
			return false
		}
		return true
	case CircuitHalfOpen:
		return false
	case CircuitClosed:
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.failureCount = 0
	cb.state = CircuitClosed

	// Record state transition if state changed
	if oldState != CircuitClosed {
		// Note: We can't get URL/protocol here, so we'll record this in the main Check function
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		// Half-open circuit fails, go back to open
		cb.state = CircuitOpen
	} else if cb.state == CircuitClosed && cb.failureCount >= cb.threshold {
		// Closed circuit reaches threshold, open it
		cb.state = CircuitOpen
	}

	// Record state transition if state changed
	if oldState != cb.state {
		// Note: We can't get URL/protocol here, so we'll record this in the main Check function
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if we need to transition from open to half-open
	if cb.state == CircuitOpen && time.Since(cb.lastFailure) >= cb.timeout {
		cb.state = CircuitHalfOpen
	}

	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// GetLastFailure returns the time of the last failure
func (cb *CircuitBreaker) GetLastFailure() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastFailure
}

// ExporterState manages thread-safe state storage for the exporter
type ExporterState struct {
	states          map[string]*URLState
	circuitBreakers map[string]*CircuitBreaker
	mu              sync.RWMutex
}

// NewExporterState creates a new thread-safe exporter state
func NewExporterState() *ExporterState {
	return &ExporterState{
		states:          make(map[string]*URLState),
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// UpdateState updates the state for a URL
func (es *ExporterState) UpdateState(url, protocol string, isUp bool, responseTime float64) {
	es.mu.Lock()
	defer es.mu.Unlock()

	key := fmt.Sprintf("%s:%s", url, protocol)
	now := time.Now()

	if state, exists := es.states[key]; exists {
		state.LastCheck = now
		state.ResponseTime = responseTime
		state.IsUp = isUp
		state.CheckCount++

		if isUp {
			state.LastSuccess = now
		} else {
			state.LastFailure = now
			state.FailureCount++
		}
	} else {
		es.states[key] = &URLState{
			URL:          url,
			Protocol:     protocol,
			LastCheck:    now,
			ResponseTime: responseTime,
			IsUp:         isUp,
			CheckCount:   1,
		}

		if isUp {
			es.states[key].LastSuccess = now
		} else {
			es.states[key].LastFailure = now
			es.states[key].FailureCount = 1
		}
	}
}

// GetState returns the current state for a URL
func (es *ExporterState) GetState(url, protocol string) (*URLState, bool) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", url, protocol)
	state, exists := es.states[key]
	return state, exists
}

// GetAllStates returns all current states
func (es *ExporterState) GetAllStates() map[string]*URLState {
	es.mu.RLock()
	defer es.mu.RUnlock()

	result := make(map[string]*URLState)
	for k, v := range es.states {
		result[k] = v
	}
	return result
}

// GetOrCreateCircuitBreaker gets or creates a circuit breaker for a URL
func (es *ExporterState) GetOrCreateCircuitBreaker(url, protocol string, threshold int, timeout time.Duration) *CircuitBreaker {
	es.mu.Lock()
	defer es.mu.Unlock()

	key := fmt.Sprintf("%s:%s", url, protocol)
	if cb, exists := es.circuitBreakers[key]; exists {
		return cb
	}

	// Create new circuit breaker
	cb := NewCircuitBreaker(threshold, timeout)
	es.circuitBreakers[key] = cb
	return cb
}

// GetCircuitBreaker gets a circuit breaker for a URL (returns nil if not found)
func (es *ExporterState) GetCircuitBreaker(url, protocol string) *CircuitBreaker {
	es.mu.RLock()
	defer es.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", url, protocol)
	return es.circuitBreakers[key]
}

// New initializes the Search struct
func New(url, port, protocol, t string, warnThreshold, critThreshold time.Duration, retryCount int, retryDelay, circuitThreshold, circuitTimeout time.Duration) (*Search, error) {

	timeout, err := time.ParseDuration(t)
	if err != nil {
		return nil, errors.New("invalid timeout, please check how to use this functional")
	}

	return &Search{
		Url:              url,
		Port:             port,
		Protocol:         protocol,
		Timeout:          timeout,
		WarnThreshold:    warnThreshold,
		CritThreshold:    critThreshold,
		RetryCount:       retryCount,
		RetryDelay:       retryDelay,
		CircuitThreshold: int(circuitThreshold.Seconds()),
		CircuitTimeout:   circuitTimeout,
	}, nil
}

func importFromFile(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("Cannot open file: " + filename)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return lines, nil
}

// importFromFileWithGroups parses a file with group configuration
// Supports [group:name] sections and URLs under each group
func importFromFileWithGroups(filename string) ([]URLWithGroup, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("Cannot open file: " + filename)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	urlsWithGroups := make([]URLWithGroup, 0)
	currentGroup := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if this is a group header [group:name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			groupContent := strings.Trim(line, "[]")
			if strings.Contains(groupContent, ":") {
				parts := strings.SplitN(groupContent, ":", 2)
				if len(parts) == 2 {
					currentGroup = strings.TrimSpace(parts[1])
					continue
				}
			}
		}

		// If it's not a group header, treat it as a URL
		if line != "" {
			urlsWithGroups = append(urlsWithGroups, URLWithGroup{
				URL:   line,
				Group: currentGroup,
			})
		}
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return urlsWithGroups, nil
}

func main() {
	url := flag.String("url", "", "a url to checking, ex: example.com")
	port := flag.String("port", "80", "a port for checking, ex: 443")
	protocol := flag.String("protocol", "tcp", "a type of protocol (tcp or udp), ex: udp")
	timeout := flag.String("timeout", "5s", "a timeout for checking in seconds, ex: 3s")
	listFromFile := flag.String("file", "", "Import urls from file, ex: urls.txt")
	jsonOutput := flag.Bool("json", false, "JSON output")
	versionFlag := flag.Bool("version", false, "Version")
	enableMetrics := flag.Bool("metrics", false, "Enable Prometheus metrics server (basic mode)")
	enableExporter := flag.Bool("exporter", false, "Enable Prometheus exporter mode with worker pool (includes metrics)")
	metricsPort := flag.Int("metrics-port", 9090, "Port for Prometheus metrics endpoint")
	checkInterval := flag.Duration("check-interval", 30*time.Second, "Interval between health checks when running in metrics mode")
	workerCount := flag.Int("workers", 5, "Number of worker goroutines for exporter mode")
	groupName := flag.String("group", "", "Group name for URL health checks")
	warnThreshold := flag.Duration("warn-threshold", 500*time.Millisecond, "Warning threshold for response time")
	critThreshold := flag.Duration("crit-threshold", 1*time.Second, "Critical threshold for response time")
	retryCount := flag.Int("retry-count", 3, "Number of retry attempts for failed checks")
	retryDelay := flag.Duration("retry-delay", 1*time.Second, "Initial delay between retry attempts")
	circuitThreshold := flag.Int("circuit-threshold", 5, "Number of consecutive failures before opening circuit breaker")
	circuitTimeout := flag.Duration("circuit-timeout", 60*time.Second, "Time to wait before attempting to close circuit breaker")
	configFile := flag.String("config", "", "Path to configuration file (YAML or JSON format)")
	flag.Parse()

	// Load configuration from file if specified
	var fileConfig *config.Config
	if *configFile != "" {
		var err error
		fileConfig, err = config.LoadConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load configuration file %s: %v", *configFile, err)
		}
		log.Printf("Configuration loaded from: %s", *configFile)
	}

	// Start with file config as base, or create default config
	var finalConfig *config.Config
	if fileConfig != nil {
		finalConfig = fileConfig
	} else {
		finalConfig = config.DefaultConfig()
	}

	// Create CLI overrides and merge them (CLI takes precedence)
	cliOverrides := &config.Config{
		URLs:                    []string{*url},
		Port:                    *port,
		Protocol:                *protocol,
		Timeout:                 *timeout,
		File:                    *listFromFile,
		JSONOutput:              *jsonOutput,
		Metrics:                 *enableMetrics,
		Exporter:                *enableExporter,
		MetricsPort:             *metricsPort,
		CheckInterval:           checkInterval.String(),
		Workers:                 *workerCount,
		GroupName:               *groupName,
		WarningThreshold:        warnThreshold.String(),
		CriticalThreshold:       critThreshold.String(),
		RetryCount:              *retryCount,
		RetryDelay:              retryDelay.String(),
		CircuitBreakerThreshold: *circuitThreshold,
		CircuitBreakerTimeout:   circuitTimeout.String(),
	}

	// Merge CLI overrides into final config (CLI takes precedence)
	finalConfig.Merge(cliOverrides)

	// Parse durations from string values
	warnThresholdDuration, err := time.ParseDuration(finalConfig.WarningThreshold)
	if err != nil {
		log.Fatalf("Invalid warn threshold value: %v", err)
	}
	critThresholdDuration, err := time.ParseDuration(finalConfig.CriticalThreshold)
	if err != nil {
		log.Fatalf("Invalid crit threshold value: %v", err)
	}
	retryDelayDuration, err := time.ParseDuration(finalConfig.RetryDelay)
	if err != nil {
		log.Fatalf("Invalid retry delay value: %v", err)
	}
	circuitTimeoutDuration, err := time.ParseDuration(finalConfig.CircuitBreakerTimeout)
	if err != nil {
		log.Fatalf("Invalid circuit timeout value: %v", err)
	}

	// Get the URL from config (either from URLs list or from File)
	var urlToUse string
	if len(finalConfig.URLs) > 0 {
		urlToUse = finalConfig.URLs[0] // Use first URL from list
	}

	search, err := New(urlToUse, finalConfig.Port, finalConfig.Protocol, finalConfig.Timeout, warnThresholdDuration, critThresholdDuration, finalConfig.RetryCount, retryDelayDuration, time.Duration(finalConfig.CircuitBreakerThreshold)*time.Second, circuitTimeoutDuration)

	if err != nil {
		log.Fatal("We can proceed, because of error: ", err)
	}

	var (
		urlsWithGroups []URLWithGroup
		wg             sync.WaitGroup
		mu             sync.Mutex
	)

	switch {
	case *versionFlag:
		version.App()
		return
	case finalConfig.File != "":
		// Use group-aware file import if group is specified, otherwise use simple import
		if finalConfig.GroupName != "" {
			// Use simple import with CLI group flag
			urlList, err := importFromFile(finalConfig.File)
			if err != nil {
				log.Fatal(err)
			}
			for _, url := range urlList {
				urlsWithGroups = append(urlsWithGroups, URLWithGroup{
					URL:   strings.TrimSpace(url),
					Group: finalConfig.GroupName,
				})
			}
		} else {
			// Use group-aware file import
			groupedURLs, err := importFromFileWithGroups(finalConfig.File)
			if err != nil {
				log.Fatal(err)
			}
			urlsWithGroups = append(urlsWithGroups, groupedURLs...)
		}

	case len(finalConfig.URLs) > 0:
		// Process URLs from configuration
		for _, url := range finalConfig.URLs {
			urlsWithGroups = append(urlsWithGroups, URLWithGroup{
				URL:   strings.TrimSpace(url),
				Group: finalConfig.GroupName,
			})
		}

	default:
		help.Show()
		return
	}

	// If exporter mode is enabled, run with worker pool (includes metrics by default)
	if finalConfig.Exporter {
		log.Printf("Starting Prometheus exporter mode with %d workers", finalConfig.Workers)
		log.Printf("Monitoring URLs: %v", getURLList(urlsWithGroups))
		if finalConfig.GroupName != "" {
			log.Printf("Group: %s", finalConfig.GroupName)
		}

		// Parse check interval from string
		checkIntervalDuration, err := time.ParseDuration(finalConfig.CheckInterval)
		if err != nil {
			log.Fatalf("Invalid check interval value: %v", err)
		}

		log.Printf("Check interval: %v", checkIntervalDuration)
		log.Printf("Metrics endpoint: http://localhost:%d/metrics", finalConfig.MetricsPort)
		log.Println("Press Ctrl+C to stop exporter...")

		// Create exporter state and worker pool
		exporterState := NewExporterState()
		workerPool := NewWorkerPool(finalConfig.Workers, exporterState, search)

		// Start worker pool
		workerPool.Start()

		// Start metrics server (included in exporter mode)
		go startMetricsServer(finalConfig.MetricsPort)
		log.Printf("Prometheus metrics server started on port %d", finalConfig.MetricsPort)

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(checkIntervalDuration)
		defer ticker.Stop()

		// Run initial checks immediately
		for _, urlWithGroup := range urlsWithGroups {
			job := CheckJob{
				URL:      urlWithGroup.URL,
				Protocol: search.Protocol,
				Search:   search,
			}
			workerPool.AddJob(job)
		}

		// Continuous monitoring loop
		for {
			select {
			case <-ticker.C:
				// Add jobs for all URLs
				for _, urlWithGroup := range urlsWithGroups {
					job := CheckJob{
						URL:      urlWithGroup.URL,
						Protocol: search.Protocol,
						Search:   search,
					}
					workerPool.AddJob(job)
				}
			case <-sigChan:
				log.Println("Received shutdown signal, stopping exporter...")
				workerPool.Stop()
				return
			}
		}
	} else if finalConfig.Metrics {
		// If metrics mode is enabled, run continuous monitoring (original behavior)
		log.Printf("Starting continuous monitoring with %d second intervals", int(checkInterval.Seconds()))
		log.Printf("Monitoring URLs: %v", getURLList(urlsWithGroups))
		if finalConfig.GroupName != "" {
			log.Printf("Group: %s", finalConfig.GroupName)
		}
		log.Println("Press Ctrl+C to stop monitoring...")

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(*checkInterval)
		defer ticker.Stop()

		// Run initial check immediately
		runHealthChecks(search, urlsWithGroups, finalConfig.JSONOutput, &wg, &mu, nil)

		// Continuous monitoring loop
		for {
			select {
			case <-ticker.C:
				runHealthChecks(search, urlsWithGroups, finalConfig.JSONOutput, &wg, &mu, nil)
			case <-sigChan:
				log.Println("Received shutdown signal, stopping monitoring...")
				return
			}
		}
	} else {
		// Run single check and exit (original behavior)
		runHealthChecks(search, urlsWithGroups, finalConfig.JSONOutput, &wg, &mu, nil)
	}
}

// retryWithExponentialBackoff performs a connection attempt with retry logic
func (search *Search) retryWithExponentialBackoff(addr string) (time.Duration, error) {
	var lastErr error
	startTime := time.Now()

	for attempt := 0; attempt <= search.RetryCount; attempt++ {
		// Attempt the connection
		_, err := net.DialTimeout(search.Protocol, addr, search.Timeout)

		if err == nil {
			// Success - return the total time taken
			return time.Since(startTime), nil
		}

		lastErr = err

		// If this is not the last attempt, wait before retrying
		if attempt < search.RetryCount {
			// Calculate exponential backoff with jitter
			delay := search.RetryDelay * time.Duration(1<<attempt) // 2^attempt

			// Add jitter (±10% of delay) to prevent thundering herd
			jitter := time.Duration(float64(delay) * 0.1 * (rand.Float64()*2 - 1))
			delay += jitter

			// Ensure delay doesn't exceed timeout
			if delay > search.Timeout {
				delay = search.Timeout / 2
			}

			log.Printf("Retry attempt %d/%d for %s after %v delay: %v",
				attempt+1, search.RetryCount, addr, delay, err)

			// Record retry attempt metric
			metrics.RecordRetryAttempt(search.SearchResult.Address, search.Protocol)

			time.Sleep(delay)
		}
	}

	// All retries failed - return the total time and last error
	return time.Since(startTime), lastErr
}

// Check - checks url address using port number with retry logic and circuit breaker
func (search *Search) Check(url string, exporterState *ExporterState) string {
	startTime := time.Now()

	var port_from_url []string = strings.Split(url, ":")

	if len(port_from_url) != 1 {
		search.SearchResult.Address = port_from_url[0]
		search.SearchResult.Port = port_from_url[1]
	} else {
		search.SearchResult.Address = url
		search.SearchResult.Port = search.Port
	}

	addr := search.SearchResult.Address + ":" + search.SearchResult.Port

	// Check circuit breaker if available
	var circuitBreaker *CircuitBreaker
	if exporterState != nil {
		circuitBreaker = exporterState.GetOrCreateCircuitBreaker(
			search.SearchResult.Address,
			search.Protocol,
			search.CircuitThreshold,
			search.CircuitTimeout,
		)

		// Check if circuit is open
		if circuitBreaker.IsOpen() {
			responseTime := time.Since(startTime)
			responseTimeSeconds := responseTime.Seconds()
			search.SearchResult.ResponseTime = responseTimeSeconds
			search.SearchResult.State = "CircuitOpen"

			// Record metrics for circuit open
			metrics.RecordCheck(addr, search.Protocol, false, responseTimeSeconds)
			metrics.RecordCheckDuration(addr, search.Protocol, responseTimeSeconds)

			return fmt.Sprintf("🚫 [Circuit Open] [%v]  %v (%.3fs)", search.Protocol, addr, responseTimeSeconds)
		}
	}

	// Use retry logic if retry count is greater than 0
	var err error
	var responseTime time.Duration

	if search.RetryCount > 0 {
		responseTime, err = search.retryWithExponentialBackoff(addr)
	} else {
		// Original behavior without retries
		responseTime = time.Since(startTime)
		_, err = net.DialTimeout(search.Protocol, addr, search.Timeout)
	}

	responseTimeSeconds := responseTime.Seconds()

	// Store response time in SearchResult (in seconds)
	search.SearchResult.ResponseTime = responseTimeSeconds

	// Update circuit breaker state and record metrics
	if circuitBreaker != nil {
		oldState := circuitBreaker.GetState()

		if err != nil {
			circuitBreaker.RecordFailure()
		} else {
			circuitBreaker.RecordSuccess()
		}

		// Record circuit breaker metrics
		newState := circuitBreaker.GetState()
		newFailureCount := circuitBreaker.GetFailureCount()

		// Record current state
		metrics.RecordCircuitBreakerState(search.SearchResult.Address, search.Protocol, int(newState))

		// Record failure count
		metrics.RecordCircuitBreakerFailureCount(search.SearchResult.Address, search.Protocol, newFailureCount)

		// Record state transitions
		if oldState != newState {
			transition := fmt.Sprintf("%s_to_%s", oldState.String(), newState.String())
			metrics.RecordCircuitBreakerTransition(search.SearchResult.Address, search.Protocol, transition)
		}
	}

	if err != nil {
		search.SearchResult.State = "Failed"
		// Record metrics for failed check
		metrics.RecordCheck(addr, search.Protocol, false, responseTimeSeconds)
		metrics.RecordCheckDuration(addr, search.Protocol, responseTimeSeconds)
		return fmt.Sprintf("😿 [-] [%v]  %v (%.3fs)", search.Protocol, addr, responseTimeSeconds)
	} else {
		search.SearchResult.State = "Success"
		// Record metrics for successful check
		metrics.RecordCheck(addr, search.Protocol, true, responseTimeSeconds)
		metrics.RecordCheckDuration(addr, search.Protocol, responseTimeSeconds)

		// Determine status based on response time thresholds
		var status string
		if responseTime > search.CritThreshold {
			status = "🔴" // Red for critical
		} else if responseTime > search.WarnThreshold {
			status = "🟡" // Yellow for warning
		} else {
			status = "🟢" // Green for normal
		}

		return fmt.Sprintf("%s [+] [%v]  %v (%.3fs)", status, search.Protocol, addr, responseTimeSeconds)
	}
}

// startMetricsServer starts the Prometheus metrics HTTP server
func startMetricsServer(port int) {
	http.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.DefaultServeMux,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down metrics server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down metrics server: %v", err)
		}
	}()

	log.Printf("Starting metrics server on port %d", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Metrics server error: %v", err)
	}
}

// runHealthChecks performs health checks on the provided URLs
func runHealthChecks(search *Search, urlsWithGroups []URLWithGroup, jsonOutput bool, wg *sync.WaitGroup, mu *sync.Mutex, exporterState *ExporterState) {
	checkResults := make(map[string]bool)
	urlResults := make(map[string]*SearchResult)
	resultsMutex := sync.Mutex{}

	for _, urlWithGroup := range urlsWithGroups {
		wg.Add(1)
		go func(urlWithGroup URLWithGroup) {
			resultText := search.Check(urlWithGroup.URL, exporterState)

			// Create result for this URL
			result := &SearchResult{
				Address:      search.SearchResult.Address,
				Port:         search.SearchResult.Port,
				State:        search.SearchResult.State,
				ResponseTime: search.SearchResult.ResponseTime,
				Group:        urlWithGroup.Group,
			}

			// Track the results for group health calculation
			resultsMutex.Lock()
			checkResults[urlWithGroup.URL] = search.SearchResult.State == "Success"
			urlResults[urlWithGroup.URL] = result
			resultsMutex.Unlock()

			if jsonOutput {
				// For backward compatibility, still output individual URL results
				resultJson, err := json.Marshal(*result)
				if err != nil {
					fmt.Println("Error:", err)
				}
				fmt.Println(string(resultJson))
			} else {
				fmt.Println(resultText)
			}

			wg.Done()
		}(urlWithGroup)
	}
	wg.Wait()

	// Calculate and display group health if there are groups
	groups := getAllGroups(urlsWithGroups)
	if len(groups) > 0 {
		fmt.Println("\n=== Group Health Summary ===")
		for _, groupName := range groups {
			// Skip empty groups in the summary
			if groupName == "" {
				continue
			}
			groupHealth := calculateGroupHealth(groupName, urlsWithGroups, checkResults)
			status := "🟢"
			if !groupHealth.IsHealthy {
				status = "🔴"
			}
			fmt.Printf("%s Group '%s': %d/%d URLs healthy\n",
				status, groupHealth.GroupName, groupHealth.HealthyURLs, groupHealth.TotalURLs)

			// Record group-level metrics
			metrics.RecordGroupHealth(groupHealth.GroupName, groupHealth.IsHealthy,
				groupHealth.TotalURLs, groupHealth.HealthyURLs)
		}
	}

	// Output nested JSON structure if requested
	if jsonOutput {
		outputNestedJSON(urlsWithGroups, checkResults, urlResults)
	}
}

// CheckJob represents a URL check job for the worker pool
type CheckJob struct {
	URL      string
	Protocol string
	Search   *Search
}

// WorkerPool manages a pool of workers for URL checking
type WorkerPool struct {
	workers  int
	jobQueue chan CheckJob
	state    *ExporterState
	search   *Search
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int, state *ExporterState, search *Search) *WorkerPool {
	return &WorkerPool{
		workers:  workers,
		jobQueue: make(chan CheckJob, workers*2),
		state:    state,
		search:   search,
		stopChan: make(chan struct{}),
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Stop stops the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.stopChan)
	wp.wg.Wait()
}

// AddJob adds a job to the worker pool
func (wp *WorkerPool) AddJob(job CheckJob) {
	select {
	case wp.jobQueue <- job:
	case <-wp.stopChan:
	}
}

// worker is the main worker function
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case job := <-wp.jobQueue:
			wp.processJob(job, id)
		case <-wp.stopChan:
			return
		}
	}
}

// processJob processes a single URL check job
func (wp *WorkerPool) processJob(job CheckJob, workerID int) {
	startTime := time.Now()

	// Parse URL to get address and port
	var port_from_url []string = strings.Split(job.URL, ":")
	var addr string

	if len(port_from_url) != 1 {
		addr = job.URL
	} else {
		addr = job.URL + ":" + wp.search.Port
	}

	// Perform the check
	timeout := wp.search.Timeout
	_, err := net.DialTimeout(job.Protocol, addr, timeout)

	// Calculate response time
	responseTime := time.Since(startTime).Seconds()

	// Determine if check was successful
	isUp := err == nil

	// Update state
	wp.state.UpdateState(job.URL, job.Protocol, isUp, responseTime)

	// Record metrics
	metrics.RecordCheck(addr, job.Protocol, isUp, responseTime)
	metrics.RecordCheckDuration(addr, job.Protocol, responseTime)

	// Log the result
	if isUp {
		log.Printf("Worker %d: ✅ [%s] %s (%.3fs)", workerID, job.Protocol, addr, responseTime)
	} else {
		log.Printf("Worker %d: ❌ [%s] %s (%.3fs) - %v", workerID, job.Protocol, addr, responseTime, err)
	}
}

// getURLList extracts URLs from URLWithGroup slice
func getURLList(urlsWithGroups []URLWithGroup) []string {
	var urls []string
	for _, urlWithGroup := range urlsWithGroups {
		urls = append(urls, urlWithGroup.URL)
	}
	return urls
}

// calculateGroupHealth calculates the health status of a group based on URL check results
func calculateGroupHealth(groupName string, urlsWithGroups []URLWithGroup, checkResults map[string]bool) *GroupStatus {
	groupURLs := make([]string, 0)
	healthyCount := 0
	totalCount := 0

	// Collect URLs for this group and count healthy ones
	for _, urlWithGroup := range urlsWithGroups {
		if urlWithGroup.Group == groupName {
			groupURLs = append(groupURLs, urlWithGroup.URL)
			totalCount++

			// Check if this URL is healthy (assuming it's in the results)
			if isHealthy, exists := checkResults[urlWithGroup.URL]; exists && isHealthy {
				healthyCount++
			}
		}
	}

	// Calculate group health (group is healthy only if all URLs are healthy)
	isHealthy := totalCount > 0 && healthyCount == totalCount

	return &GroupStatus{
		GroupName:     groupName,
		IsHealthy:     isHealthy,
		TotalURLs:     totalCount,
		HealthyURLs:   healthyCount,
		UnhealthyURLs: totalCount - healthyCount,
		URLs:          groupURLs,
	}
}

// getAllGroups returns all unique group names from URLs
func getAllGroups(urlsWithGroups []URLWithGroup) []string {
	groupMap := make(map[string]bool)
	var groups []string

	for _, urlWithGroup := range urlsWithGroups {
		if !groupMap[urlWithGroup.Group] {
			groupMap[urlWithGroup.Group] = true
			groups = append(groups, urlWithGroup.Group)
		}
	}

	return groups
}

// outputNestedJSON outputs the nested JSON structure with groups
func outputNestedJSON(urlsWithGroups []URLWithGroup, checkResults map[string]bool, urlResults map[string]*SearchResult) {
	result := HealthCheckResult{}

	// Group URLs by their group name
	groupedURLs := make(map[string][]*SearchResult)
	ungroupedURLs := make([]*SearchResult, 0)

	for _, urlWithGroup := range urlsWithGroups {
		if urlResult, exists := urlResults[urlWithGroup.URL]; exists {
			if urlWithGroup.Group == "" {
				ungroupedURLs = append(ungroupedURLs, urlResult)
			} else {
				groupedURLs[urlWithGroup.Group] = append(groupedURLs[urlWithGroup.Group], urlResult)
			}
		}
	}

	// Create group results
	for groupName, urls := range groupedURLs {
		groupHealth := calculateGroupHealth(groupName, urlsWithGroups, checkResults)

		groupResult := GroupResult{
			GroupName:     groupName,
			IsHealthy:     groupHealth.IsHealthy,
			TotalURLs:     groupHealth.TotalURLs,
			HealthyURLs:   groupHealth.HealthyURLs,
			UnhealthyURLs: groupHealth.UnhealthyURLs,
			URLs:          make([]SearchResult, len(urls)),
		}

		for i, url := range urls {
			groupResult.URLs[i] = *url
		}

		result.Groups = append(result.Groups, groupResult)
	}

	// Add ungrouped URLs
	if len(ungroupedURLs) > 0 {
		result.UngroupedURLs = make([]SearchResult, len(ungroupedURLs))
		for i, url := range ungroupedURLs {
			result.UngroupedURLs[i] = *url
		}
	}

	// Calculate summary
	healthyGroups := 0
	healthyURLs := 0
	totalURLs := 0

	for _, group := range result.Groups {
		if group.IsHealthy {
			healthyGroups++
		}
		healthyURLs += group.HealthyURLs
		totalURLs += group.TotalURLs
	}

	// Add ungrouped URLs to summary
	totalURLs += len(result.UngroupedURLs)
	for _, url := range result.UngroupedURLs {
		if url.State == "Success" {
			healthyURLs++
		}
	}

	result.Summary.TotalGroups = len(result.Groups)
	result.Summary.HealthyGroups = healthyGroups
	result.Summary.UnhealthyGroups = len(result.Groups) - healthyGroups
	result.Summary.TotalURLs = totalURLs
	result.Summary.HealthyURLs = healthyURLs
	result.Summary.UnhealthyURLs = totalURLs - healthyURLs

	// Output the nested JSON structure
	nestedJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling nested JSON:", err)
		return
	}

	fmt.Println("\n=== Nested Group Structure ===")
	fmt.Println(string(nestedJson))
}
