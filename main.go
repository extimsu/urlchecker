package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/extimsu/urlchecker/help"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/extimsu/urlchecker/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Search struct {
	Url      string
	Port     string
	Protocol string
	Timeout  time.Duration
	SearchResult
}

type SearchResult struct {
	Address string `json:"address"`
	Port    string `json:"port"`
	State   string `json:"state"`
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

// ExporterState manages thread-safe state storage for the exporter
type ExporterState struct {
	states map[string]*URLState
	mu     sync.RWMutex
}

// NewExporterState creates a new thread-safe exporter state
func NewExporterState() *ExporterState {
	return &ExporterState{
		states: make(map[string]*URLState),
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

// New initializes the Search struct
func New(url, port, protocol, t string) (*Search, error) {

	timeout, err := time.ParseDuration(t)
	if err != nil {
		return nil, errors.New("invalid timeout, please check how to use this functional")
	}

	return &Search{
		Url:      url,
		Port:     port,
		Protocol: protocol,
		Timeout:  timeout,
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
	flag.Parse()

	// Start metrics server if enabled
	if *enableMetrics {
		go startMetricsServer(*metricsPort)
		log.Printf("Prometheus metrics server started on port %d", *metricsPort)
	}

	search, err := New(*url, *port, *protocol, *timeout)

	if err != nil {
		log.Fatal("We can proceed, because of error: ", err)
	}

	var (
		urls []string
		wg   sync.WaitGroup
		mu   sync.Mutex
	)

	switch {
	case *versionFlag:
		version.App()
		return
	case *listFromFile != "":
		urls, err = importFromFile(*listFromFile)
		if err != nil {
			log.Fatal(err)
		}

	case search.Url != "":
		urls = strings.Split(search.Url, ",")

	default:
		help.Show()
		return
	}

	// If exporter mode is enabled, run with worker pool (includes metrics by default)
	if *enableExporter {
		log.Printf("Starting Prometheus exporter mode with %d workers", *workerCount)
		log.Printf("Monitoring URLs: %v", urls)
		log.Printf("Check interval: %v", *checkInterval)
		log.Printf("Metrics endpoint: http://localhost:%d/metrics", *metricsPort)
		log.Println("Press Ctrl+C to stop exporter...")

		// Create exporter state and worker pool
		exporterState := NewExporterState()
		workerPool := NewWorkerPool(*workerCount, exporterState, search)

		// Start worker pool
		workerPool.Start()

		// Start metrics server (included in exporter mode)
		go startMetricsServer(*metricsPort)
		log.Printf("Prometheus metrics server started on port %d", *metricsPort)

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(*checkInterval)
		defer ticker.Stop()

		// Run initial checks immediately
		for _, url := range urls {
			job := CheckJob{
				URL:      url,
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
				for _, url := range urls {
					job := CheckJob{
						URL:      url,
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
	} else if *enableMetrics {
		// If metrics mode is enabled, run continuous monitoring (original behavior)
		log.Printf("Starting continuous monitoring with %d second intervals", int(checkInterval.Seconds()))
		log.Printf("Monitoring URLs: %v", urls)
		log.Println("Press Ctrl+C to stop monitoring...")

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(*checkInterval)
		defer ticker.Stop()

		// Run initial check immediately
		runHealthChecks(search, urls, *jsonOutput, &wg, &mu)

		// Continuous monitoring loop
		for {
			select {
			case <-ticker.C:
				runHealthChecks(search, urls, *jsonOutput, &wg, &mu)
			case <-sigChan:
				log.Println("Received shutdown signal, stopping monitoring...")
				return
			}
		}
	} else {
		// Run single check and exit (original behavior)
		runHealthChecks(search, urls, *jsonOutput, &wg, &mu)
	}
}

// Check - checks url address using port number
func (search *Search) Check(url string) string {
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
	timeout := search.Timeout
	_, err := net.DialTimeout(search.Protocol, addr, timeout)

	// Calculate response time
	responseTime := time.Since(startTime).Seconds()

	if err != nil {
		search.SearchResult.State = "Failed"
		// Record metrics for failed check
		metrics.RecordCheck(addr, search.Protocol, false, responseTime)
		metrics.RecordCheckDuration(addr, search.Protocol, responseTime)
		return fmt.Sprintf("😿 [-] [%v]  %v", search.Protocol, addr)
	} else {
		search.SearchResult.State = "Success"
		// Record metrics for successful check
		metrics.RecordCheck(addr, search.Protocol, true, responseTime)
		metrics.RecordCheckDuration(addr, search.Protocol, responseTime)
		return fmt.Sprintf("😺 [+] [%v]  %v", search.Protocol, addr)
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
func runHealthChecks(search *Search, urls []string, jsonOutput bool, wg *sync.WaitGroup, mu *sync.Mutex) {
	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			mu.Lock()
			defer mu.Unlock()

			resultText := search.Check(url)

			if jsonOutput {
				result := &SearchResult{
					Address: search.SearchResult.Address,
					Port:    search.SearchResult.Port,
					State:   search.SearchResult.State,
				}

				resultJson, err := json.Marshal(*result)
				if err != nil {
					fmt.Println("Error:", err)
				}
				fmt.Println(string(resultJson))
			} else {
				fmt.Println(resultText)
			}

			wg.Done()
		}(url)
	}
	wg.Wait()
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
