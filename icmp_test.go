package main

import (
	"testing"
	"time"

	"github.com/extimsu/urlchecker/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestSplitScheme(t *testing.T) {
	for _, c := range []struct{ in, proto, rest string }{
		{"icmp://10.0.0.1", "icmp", "10.0.0.1"},
		{"TCP://example.com:443", "tcp", "example.com:443"},
		{"udp://10.0.0.1:53", "udp", "10.0.0.1:53"},
		{"example.com:443", "tcp", "example.com:443"},
	} {
		proto, rest := splitScheme(c.in, "tcp")
		if proto != c.proto || rest != c.rest {
			t.Errorf("splitScheme(%q) = %q, %q; want %q, %q", c.in, proto, rest, c.proto, c.rest)
		}
	}
}

// skipWithoutICMP skips when the test process may open neither a raw nor an
// unprivileged ICMP socket (no CAP_NET_RAW and GID outside ping_group_range).
func skipWithoutICMP(t *testing.T) {
	t.Helper()
	p, err := newPinger("127.0.0.1")
	if err != nil {
		t.Skipf("no ICMP socket here: %v", err)
	}
	p.close()
}

func TestPingOnce(t *testing.T) {
	skipWithoutICMP(t)
	if err := pingOnce("127.0.0.1", time.Second); err != nil {
		t.Fatalf("ping 127.0.0.1: %v", err)
	}
	// 192.0.2.0/24 is TEST-NET-1: nothing answers there.
	if err := pingOnce("192.0.2.1:80", 200*time.Millisecond); err == nil {
		t.Fatal("ping 192.0.2.1 succeeded, want a timeout")
	}
}

func TestCheckICMP(t *testing.T) {
	skipWithoutICMP(t)
	s, err := New("", "80", "tcp", "1s", time.Second, 2*time.Second, 0, time.Second, 5*time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s.Check("icmp://127.0.0.1", nil)
	if s.SearchResult.State != "Success" || s.SearchResult.Port != "" {
		t.Fatalf("Check(icmp://127.0.0.1) = state %q port %q, want Success and no port", s.SearchResult.State, s.SearchResult.Port)
	}
}

// replies returns how many echo replies the histogram counted for host/ip.
func replies(t *testing.T, host, ip string) uint64 {
	t.Helper()
	var m dto.Metric
	if err := metrics.ICMPResponseDuration.WithLabelValues(host, ip).(prometheus.Metric).Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestRunPingerCountsLoss(t *testing.T) {
	skipWithoutICMP(t)
	metrics.ResetMetrics()

	run := func(host string) {
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() { runPinger(host, 50*time.Millisecond, time.Second, stop); close(done) }()
		time.Sleep(300 * time.Millisecond)
		close(stop)
		<-done
	}
	run("127.0.0.1")
	run("192.0.2.1")

	sent := testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("127.0.0.1", "127.0.0.1"))
	if got := replies(t, "127.0.0.1", "127.0.0.1"); sent < 3 || float64(got) != sent {
		t.Errorf("127.0.0.1: %v requests, %d replies; want every request answered", sent, got)
	}
	// The silent target has requests and a reply series at 0, so its loss is 100%, not "no data".
	sent = testutil.ToFloat64(metrics.ICMPRequests.WithLabelValues("192.0.2.1", "192.0.2.1"))
	if got := replies(t, "192.0.2.1", "192.0.2.1"); sent < 3 || got != 0 {
		t.Errorf("192.0.2.1: %v requests, %d replies; want requests and no replies", sent, got)
	}
}
