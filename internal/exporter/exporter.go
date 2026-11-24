// Package exporter runs the checks continuously for Prometheus. TCP and UDP
// targets go through a worker pool every check interval. Each ICMP target gets
// its own pinger, smokeping_prober style.
//
// The exporter never retries a failed check: a retry would hide the packet
// loss and flapping it is there to measure.
package exporter

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/extimsu/urlchecker/internal/probe"
	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
)

// Options configure an Exporter.
type Options struct {
	Workers       int           // concurrent TCP/UDP checks
	CheckInterval time.Duration // between TCP/UDP checks of one target
	PingInterval  time.Duration // between echo requests to one ICMP target
	Timeout       time.Duration // connect timeout; the echo wait is capped at PingInterval
	Logger        *slog.Logger
}

// Exporter holds the current target set and its probers.
type Exporter struct {
	opts  Options
	log   *slog.Logger
	jobs  chan *entry
	ready atomic.Bool

	mu      sync.Mutex
	ctx     context.Context // set by Run; pingers stop when it is done
	entries map[string]*entry
}

// entry is one target and its state.
type entry struct {
	t       target.Target
	stop    context.CancelFunc // ICMP only
	busy    atomic.Bool        // a TCP/UDP check is queued or running
	removed atomic.Bool

	// guarded by Exporter.mu
	checked bool
	up      bool
}

// New returns an Exporter. Call Run to start it.
func New(opts Options) *Exporter {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Exporter{
		opts:    opts,
		log:     opts.Logger,
		jobs:    make(chan *entry, 4096),
		entries: map[string]*entry{},
	}
}

// Ready reports whether Run has loaded the targets and started the probers.
func (e *Exporter) Ready() bool { return e.ready.Load() }

// Run checks targets until ctx is done. Later SetTargets calls replace the set.
func (e *Exporter) Run(ctx context.Context, targets []target.Target) {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
	e.SetTargets(targets)

	var wg sync.WaitGroup
	for range e.opts.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.worker(ctx)
		}()
	}
	e.ready.Store(true)
	e.log.Info("exporter started", "targets", len(targets), "workers", e.opts.Workers,
		"check_interval", e.opts.CheckInterval.String(), "ping_interval", e.opts.PingInterval.String())

	ticker := time.NewTicker(e.opts.CheckInterval)
	defer ticker.Stop()
	for {
		e.schedule()
		select {
		case <-ticker.C:
		case <-ctx.Done():
			e.ready.Store(false)
			wg.Wait()
			return
		}
	}
}

// SetTargets replaces the target set. Unchanged targets keep their state and
// pinger. Removed targets stop, and their series are deleted.
func (e *Exporter) SetTargets(targets []target.Target) {
	e.mu.Lock()
	defer e.mu.Unlock()

	next := make(map[string]target.Target, len(targets))
	for _, t := range targets {
		next[t.Key()] = t
	}
	groups := map[string]bool{}

	for key, en := range e.entries {
		if t, ok := next[key]; ok && t.Group == en.t.Group {
			continue
		}
		en.removed.Store(true)
		if en.stop != nil {
			en.stop()
		}
		if en.t.Protocol == "icmp" {
			metrics.DeleteICMP(en.t.Host)
		} else {
			metrics.DeleteCheck(en.t.Addr(), en.t.Protocol)
		}
		groups[en.t.Group] = true
		delete(e.entries, key)
		e.log.Info("target removed", "target", key)
	}

	for key, t := range next {
		if _, ok := e.entries[key]; ok {
			continue
		}
		en := &entry{t: t}
		e.entries[key] = en
		groups[t.Group] = true
		if t.Protocol == "icmp" && e.ctx != nil {
			ctx, cancel := context.WithCancel(e.ctx)
			en.stop = cancel
			go e.ping(ctx, en)
		}
		e.log.Debug("target added", "target", key, "group", t.Group)
	}

	for g := range groups {
		e.updateGroupLocked(g)
	}
}

// schedule queues every idle TCP/UDP target. A target whose last check is
// still running is skipped, so a slow target can't pile up checks.
func (e *Exporter) schedule() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, en := range e.entries {
		if en.t.Protocol == "icmp" || !en.busy.CompareAndSwap(false, true) {
			continue
		}
		select {
		case e.jobs <- en:
		default:
			en.busy.Store(false) // queue full: try again next round
		}
	}
}

func (e *Exporter) worker(ctx context.Context) {
	for {
		select {
		case en := <-e.jobs:
			e.check(ctx, en)
			en.busy.Store(false)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Exporter) check(ctx context.Context, en *entry) {
	if en.removed.Load() {
		return
	}
	rtt, err := probe.Check(ctx, en.t, e.opts.Timeout)
	if ctx.Err() != nil {
		return // shutting down: the failure is ours, not the target's
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if en.removed.Load() {
		return // removed while checking: don't bring its series back
	}
	metrics.RecordCheck(en.t.Addr(), en.t.Protocol, err == nil, rtt.Seconds())
	metrics.RecordCheckDuration(en.t.Addr(), en.t.Protocol, rtt.Seconds())
	e.setStateLocked(en, err)
}

// ping sends an echo request every PingInterval until ctx is done. It exports
// the same counters as smokeping_prober, so packet loss is
// 1 - rate(urlchecker_icmp_response_duration_seconds_count) / rate(urlchecker_icmp_requests_total).
func (e *Exporter) ping(ctx context.Context, en *entry) {
	timeout := min(e.opts.Timeout, e.opts.PingInterval)
	ticker := time.NewTicker(e.opts.PingInterval)
	defer ticker.Stop()

	var p *probe.Pinger
	defer func() {
		if p != nil {
			_ = p.Close()
		}
	}()
	for {
		if p == nil {
			var err error
			if p, err = probe.NewPinger(en.t.Host); err != nil {
				e.mu.Lock()
				e.setStateLocked(en, err)
				e.mu.Unlock()
			} else {
				// Create the reply series now, so a target that never answers
				// shows 100% loss, not "no data".
				metrics.ICMPResponseDuration.WithLabelValues(en.t.Host, p.IP().String())
			}
		}
		if p != nil {
			ip := p.IP().String()
			metrics.ICMPRequests.WithLabelValues(en.t.Host, ip).Inc()
			rtt, err := p.Ping(timeout)
			if err == nil {
				metrics.ICMPResponseDuration.WithLabelValues(en.t.Host, ip).Observe(rtt.Seconds())
			}
			e.mu.Lock()
			if !en.removed.Load() {
				e.setStateLocked(en, err)
			}
			e.mu.Unlock()
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// setStateLocked records the result of a check, logs a change and updates
// the target's group.
func (e *Exporter) setStateLocked(en *entry, err error) {
	up := err == nil
	if en.checked && en.up == up {
		return
	}
	first := !en.checked
	en.checked, en.up = true, up
	switch {
	case !up:
		e.log.Warn("target down", "target", en.t.Key(), "error", err)
	case !first:
		e.log.Info("target up", "target", en.t.Key())
	}
	e.updateGroupLocked(en.t.Group)
}

// updateGroupLocked sets the group metrics: healthy when every target is up.
func (e *Exporter) updateGroupLocked(group string) {
	if group == "" {
		return
	}
	total, healthy := 0, 0
	for _, en := range e.entries {
		if en.t.Group != group {
			continue
		}
		total++
		if en.checked && en.up {
			healthy++
		}
	}
	if total == 0 {
		metrics.DeleteGroup(group)
		return
	}
	metrics.RecordGroupHealth(group, healthy == total, total, healthy)
}
