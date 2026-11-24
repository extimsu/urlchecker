// Command urlchecker checks TCP, UDP and ICMP reachability, once from the
// command line or continuously as a Prometheus exporter.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/extimsu/urlchecker/config"
	"github.com/extimsu/urlchecker/help"
	"github.com/extimsu/urlchecker/internal/exporter"
	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/internal/web"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/extimsu/urlchecker/version"
)

// Exit codes.
const (
	exitOK     = 0
	exitDown   = 1 // one-shot mode: at least one target failed
	exitConfig = 2 // bad flags, config or target list
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// settings are the parsed, validated values of the config and flags.
type settings struct {
	cfg                             *config.Config
	timeout, warn, crit, retryDelay time.Duration
	checkInterval, pingInterval     time.Duration
	targets                         []target.Target
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("urlchecker", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		url            = fs.String("url", "", "targets to check, comma-separated, ex: example.com:443,icmp://10.0.0.1")
		port           = fs.String("port", "80", "port for targets without one, ex: 443")
		protocol       = fs.String("protocol", "tcp", "protocol for targets without a prefix: tcp, udp or icmp")
		timeout        = fs.String("timeout", "5s", "connect timeout, ex: 3s")
		file           = fs.String("file", "", "file with one target per line, ex: urls.txt")
		jsonOutput     = fs.Bool("json", false, "JSON output (one-shot and metrics modes)")
		versionFlag    = fs.Bool("version", false, "print the version and exit")
		enableMetrics  = fs.Bool("metrics", false, "check every --check-interval and serve /metrics (with retries)")
		enableExporter = fs.Bool("exporter", false, "Prometheus exporter: TCP/UDP every --check-interval, ICMP every --ping-interval")
		metricsPort    = fs.Int("metrics-port", 9090, "port for /metrics, /healthz and /readyz")
		checkInterval  = fs.Duration("check-interval", 30*time.Second, "interval between TCP and UDP checks")
		workers        = fs.Int("workers", 5, "concurrent TCP and UDP checks in exporter mode")
		pingInterval   = fs.Duration("ping-interval", time.Second, "interval between echo requests to each icmp:// target in exporter mode")
		group          = fs.String("group", "", "put every target in this group")
		warn           = fs.Duration("warn-threshold", 500*time.Millisecond, "response time shown as a warning (one-shot)")
		crit           = fs.Duration("crit-threshold", time.Second, "response time shown as critical (one-shot)")
		retryCount     = fs.Int("retry-count", 3, "retries after a failed check (one-shot and metrics modes)")
		retryDelay     = fs.Duration("retry-delay", time.Second, "first retry delay, doubled each retry")
		logLevel       = fs.String("log-level", "info", "log level: debug, info, warn or error")
		logFormat      = fs.String("log-format", "text", "log format: text or json")
		configFile     = fs.String("config", "", "YAML or JSON config file; flags override it")
	)
	// Deprecated in 0.3.0: accepted so existing command lines keep working.
	fs.Int("circuit-threshold", 0, "deprecated, ignored")
	fs.Duration("circuit-timeout", 0, "deprecated, ignored")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitConfig
	}
	if *versionFlag {
		version.App()
		return exitOK
	}

	cfg := config.DefaultConfig()
	if *configFile != "" {
		var err error
		if cfg, err = config.LoadConfig(*configFile); err != nil {
			return fail(stderr, err)
		}
	}
	// Only flags given on the command line override the file.
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	apply := map[string]func(){
		"url":            func() { cfg.URLs = splitList(*url) },
		"port":           func() { cfg.Port = *port },
		"protocol":       func() { cfg.Protocol = *protocol },
		"timeout":        func() { cfg.Timeout = *timeout },
		"file":           func() { cfg.File = *file },
		"json":           func() { cfg.JSONOutput = *jsonOutput },
		"metrics":        func() { cfg.Metrics = *enableMetrics },
		"exporter":       func() { cfg.Exporter = *enableExporter },
		"metrics-port":   func() { cfg.MetricsPort = *metricsPort },
		"check-interval": func() { cfg.CheckInterval = checkInterval.String() },
		"workers":        func() { cfg.Workers = *workers },
		"ping-interval":  func() { cfg.PingInterval = pingInterval.String() },
		"group":          func() { cfg.GroupName = *group },
		"warn-threshold": func() { cfg.WarningThreshold = warn.String() },
		"crit-threshold": func() { cfg.CriticalThreshold = crit.String() },
		"retry-count":    func() { cfg.RetryCount = *retryCount },
		"retry-delay":    func() { cfg.RetryDelay = retryDelay.String() },
		"log-level":      func() { cfg.LogLevel = *logLevel },
		"log-format":     func() { cfg.LogFormat = *logFormat },
	}
	for name, f := range apply {
		if set[name] {
			f()
		}
	}

	log, err := newLogger(stderr, cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return fail(stderr, err)
	}
	slog.SetDefault(log)
	for _, name := range []string{"circuit-threshold", "circuit-timeout"} {
		if set[name] {
			log.Warn("flag is deprecated and ignored: the circuit breaker was removed in 0.3.0", "flag", "--"+name)
		}
	}
	for _, key := range cfg.DeprecatedKeys() {
		log.Warn("config key is deprecated and ignored", "key", key)
	}

	s, err := resolve(cfg)
	if err != nil {
		return fail(stderr, err)
	}
	if len(s.targets) == 0 {
		help.Show(stderr)
		return exitConfig
	}
	metrics.SetBuildInfo(version.Version, version.GitCommit)

	switch {
	case cfg.Exporter:
		return runExporter(ctx, s, log)
	case cfg.Metrics:
		return runMetrics(ctx, s, stdout, log)
	default:
		if checkAll(ctx, s.checker(), s.targets, cfg.JSONOutput, stdout) {
			return exitOK
		}
		return exitDown
	}
}

// resolve parses the durations and loads the targets.
func resolve(cfg *config.Config) (*settings, error) {
	s := &settings{cfg: cfg}
	for _, d := range []struct {
		name string
		in   string
		out  *time.Duration
	}{
		{"timeout", cfg.Timeout, &s.timeout},
		{"warning threshold", cfg.WarningThreshold, &s.warn},
		{"critical threshold", cfg.CriticalThreshold, &s.crit},
		{"retry delay", cfg.RetryDelay, &s.retryDelay},
		{"check interval", cfg.CheckInterval, &s.checkInterval},
		{"ping interval", cfg.PingInterval, &s.pingInterval},
	} {
		v, err := time.ParseDuration(d.in)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", d.name, d.in, err)
		}
		*d.out = v
	}
	if s.checkInterval <= 0 || s.pingInterval <= 0 || s.timeout <= 0 {
		return nil, fmt.Errorf("timeout, check interval and ping interval must be positive")
	}

	if cfg.File != "" {
		ts, err := target.ReadFile(cfg.File, cfg.Protocol, cfg.Port, cfg.GroupName)
		if err != nil {
			return nil, err
		}
		s.targets = append(s.targets, ts...)
	}
	add := func(raw, group string) error {
		t, err := target.Parse(raw, cfg.Protocol, cfg.Port)
		if err != nil {
			return err
		}
		t.Group = group
		s.targets = append(s.targets, t)
		return nil
	}
	for _, u := range cfg.URLs {
		if err := add(u, cfg.GroupName); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Groups)) {
		for _, u := range cfg.Groups[name].URLs {
			if err := add(u, name); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}

func (s *settings) checker() checker {
	return checker{timeout: s.timeout, warn: s.warn, crit: s.crit, retries: s.cfg.RetryCount, retryDelay: s.retryDelay}
}

func runExporter(ctx context.Context, s *settings, log *slog.Logger) int {
	exp := exporter.New(exporter.Options{
		Workers:       s.cfg.Workers,
		CheckInterval: s.checkInterval,
		PingInterval:  s.pingInterval,
		Timeout:       s.timeout,
		Logger:        log,
	})
	errc := make(chan error, 1)
	go func() { errc <- web.Serve(ctx, fmt.Sprintf(":%d", s.cfg.MetricsPort), exp.Ready, log) }()
	exp.Run(ctx, s.targets)
	if err := <-errc; err != nil {
		log.Error("metrics server failed", "error", err)
		return exitConfig
	}
	log.Info("stopped")
	return exitOK
}

func runMetrics(ctx context.Context, s *settings, stdout io.Writer, log *slog.Logger) int {
	errc := make(chan error, 1)
	go func() {
		errc <- web.Serve(ctx, fmt.Sprintf(":%d", s.cfg.MetricsPort), func() bool { return true }, log)
	}()
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()
	for {
		checkAll(ctx, s.checker(), s.targets, s.cfg.JSONOutput, stdout)
		select {
		case <-ticker.C:
		case err := <-errc:
			log.Error("metrics server failed", "error", err)
			return exitConfig
		case <-ctx.Done():
			if err := <-errc; err != nil {
				log.Error("metrics server failed", "error", err)
			}
			return exitOK
		}
	}
}

func newLogger(w io.Writer, level, format string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q", level)
	}
	opts := &slog.HandlerOptions{Level: l}
	switch format {
	case "text", "":
		return slog.New(slog.NewTextHandler(w, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, opts)), nil
	}
	return nil, fmt.Errorf("invalid log format %q (want text or json)", format)
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "urlchecker:", err)
	return exitConfig
}

// splitList splits a comma-separated flag value and drops empty entries.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
