package exporter

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/probe"
	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func tcp(t *testing.T, addr, group string) target.Target {
	t.Helper()
	tg, err := target.Parse(addr, "tcp", "80")
	if err != nil {
		t.Fatal(err)
	}
	tg.Group = group
	return tg
}

// start runs an exporter until the test ends.
func start(t *testing.T, opts Options, targets []target.Target) *Exporter {
	t.Helper()
	opts.Logger = quiet()
	e := New(opts)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); e.Run(ctx, targets) }()
	t.Cleanup(func() { cancel(); wg.Wait() })
	return e
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func gauge(g *prometheus.GaugeVec, labels ...string) float64 {
	return testutil.ToFloat64(g.WithLabelValues(labels...))
}

func TestExporterChecksAndGroups(t *testing.T) {
	metrics.ResetMetrics()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dead, _ := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	closed := dead.Addr().String()
	_ = dead.Close()

	up, down := tcp(t, ln.Addr().String(), "web"), tcp(t, closed, "web")
	e := start(t, Options{Workers: 2, CheckInterval: 50 * time.Millisecond, PingInterval: time.Second, Timeout: time.Second},
		[]target.Target{up, down})

	eventually(t, "both targets checked", func() bool {
		return testutil.CollectAndCount(metrics.CurrentStatusGauge) == 2
	})
	if !e.Ready() {
		t.Error("Ready() = false while running")
	}
	if g := gauge(metrics.CurrentStatusGauge, up.Addr(), "tcp"); g != 1 {
		t.Errorf("open port: current_status %v, want 1", g)
	}
	if g := gauge(metrics.CurrentStatusGauge, down.Addr(), "tcp"); g != 0 {
		t.Errorf("closed port: current_status %v, want 0", g)
	}
	eventually(t, "group web 1/2", func() bool {
		return gauge(metrics.GroupHealthyURLsGauge, "web") == 1 && gauge(metrics.GroupTotalURLsGauge, "web") == 2
	})
	if h := gauge(metrics.GroupHealthGauge, "web"); h != 0 {
		t.Errorf("group_health %v with one target down, want 0", h)
	}

	// Removing the closed target deletes its series and makes the group healthy.
	e.SetTargets([]target.Target{up})
	if n := testutil.CollectAndCount(metrics.CurrentStatusGauge); n != 1 {
		t.Errorf("%d current_status series after removing a target, want 1", n)
	}
	if h := gauge(metrics.GroupHealthGauge, "web"); h != 1 {
		t.Errorf("group_health %v after removing the down target, want 1", h)
	}
	e.SetTargets(nil)
	if n := testutil.CollectAndCount(metrics.GroupHealthGauge); n != 0 {
		t.Errorf("%d group series with no targets left, want 0", n)
	}
}

func replies(t *testing.T, host, ip string) uint64 {
	t.Helper()
	var m dto.Metric
	if err := metrics.ICMPResponseDuration.WithLabelValues(host, ip).(prometheus.Metric).Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestExporterCountsICMPLoss(t *testing.T) {
	if p, err := probe.NewPinger("127.0.0.1"); err != nil {
		t.Skipf("no ICMP socket here: %v", err)
	} else {
		_ = p.Close()
	}
	metrics.ResetMetrics()
	lo := target.Target{Protocol: "icmp", Host: "127.0.0.1"}
	void := target.Target{Protocol: "icmp", Host: "192.0.2.1"} // TEST-NET-1: nothing answers
	e := start(t, Options{Workers: 1, CheckInterval: time.Hour, PingInterval: 50 * time.Millisecond, Timeout: time.Second},
		[]target.Target{lo, void})

	eventually(t, "a few pings each", func() bool {
		return testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("192.0.2.1", "192.0.2.1")) >= 3 &&
			testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("127.0.0.1", "127.0.0.1")) >= 3
	})
	e.SetTargets([]target.Target{lo}) // stop the void pinger before reading it
	sent := testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("127.0.0.1", "127.0.0.1"))
	if got := replies(t, "127.0.0.1", "127.0.0.1"); float64(got) < sent-1 {
		t.Errorf("127.0.0.1: %v requests, %d replies; want every request answered", sent, got)
	}
	if n := testutil.CollectAndCount(metrics.ICMPRequests); n != 1 {
		t.Errorf("%d request series after removing 192.0.2.1, want 1", n)
	}
}

func TestExporterSilentTargetShowsLoss(t *testing.T) {
	if p, err := probe.NewPinger("127.0.0.1"); err != nil {
		t.Skipf("no ICMP socket here: %v", err)
	} else {
		_ = p.Close()
	}
	metrics.ResetMetrics()
	start(t, Options{Workers: 1, CheckInterval: time.Hour, PingInterval: 50 * time.Millisecond, Timeout: time.Second},
		[]target.Target{{Protocol: "icmp", Host: "192.0.2.1"}})
	eventually(t, "requests to the silent target", func() bool {
		return testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("192.0.2.1", "192.0.2.1")) >= 3
	})
	// The reply series exists at 0, so loss is 100% rather than "no data".
	if got := replies(t, "192.0.2.1", "192.0.2.1"); got != 0 {
		t.Errorf("%d replies from TEST-NET-1, want 0", got)
	}
}
