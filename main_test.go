package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// listen returns an open local TCP port and a closed one.
func listen(t *testing.T) (open, closed string) {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dead, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed = dead.Addr().String()
	_ = dead.Close()
	return ln.Addr().String(), closed
}

func mustParse(t *testing.T, s, group string) target.Target {
	t.Helper()
	tg, err := target.Parse(s, "tcp", "80")
	if err != nil {
		t.Fatal(err)
	}
	tg.Group = group
	return tg
}

// TestCheckAllAttributesResults checks many targets in parallel under -race:
// every result must belong to its own target.
func TestCheckAllAttributesResults(t *testing.T) {
	open, closed := listen(t)
	var targets []target.Target
	for i := range 50 {
		addr := open
		if i%2 == 1 {
			addr = closed
		}
		targets = append(targets, mustParse(t, addr, fmt.Sprintf("g%d", i%2)))
	}
	var out bytes.Buffer
	c := checker{timeout: time.Second, warn: time.Second, crit: 2 * time.Second}
	if checkAll(context.Background(), c, targets, true, &out) {
		t.Fatal("checkAll reported all up, but half the targets are closed")
	}
	lines := strings.Split(out.String(), "\n")
	for i := range targets {
		var r SearchResult
		if err := json.Unmarshal([]byte(lines[i]), &r); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		wantState := map[bool]string{true: "Success", false: "Failed"}[i%2 == 0]
		if r.State != wantState || r.Group != targets[i].Group || net.JoinHostPort(r.Address, r.Port) != targets[i].Addr() {
			t.Errorf("result %d = %+v, want %s for %s in %s", i, r, wantState, targets[i].Addr(), targets[i].Group)
		}
	}
	if !strings.Contains(out.String(), "Group 'g0': 25/25 URLs healthy") || !strings.Contains(out.String(), "Group 'g1': 0/25 URLs healthy") {
		t.Errorf("group summary missing or wrong:\n%s", out.String())
	}
}

func TestCheckerRetries(t *testing.T) {
	_, closed := listen(t)
	metrics.ResetMetrics()
	c := checker{timeout: time.Second, retries: 2, retryDelay: time.Millisecond, warn: time.Second, crit: time.Second}
	tg := mustParse(t, closed, "")
	if r, _ := c.check(context.Background(), tg); r.State != "Failed" {
		t.Fatalf("state %q, want Failed", r.State)
	}
	if n := testutil.ToFloat64(metrics.RetryAttemptsCounter.WithLabelValues(tg.MetricName(), "tcp")); n != 2 {
		t.Errorf("%v retries recorded, want 2", n)
	}
	if n := testutil.ToFloat64(metrics.CurrentStatusGauge.WithLabelValues(tg.Addr(), "tcp")); n != 0 {
		t.Errorf("current_status %v, want 0", n)
	}
}

func TestGroupHealth(t *testing.T) {
	targets := []target.Target{
		mustParse(t, "a:1", "web"), mustParse(t, "b:1", "web"), mustParse(t, "c:1", "db"), mustParse(t, "d:1", ""),
	}
	up := map[string]bool{"tcp://a:1": true, "tcp://b:1": false, "tcp://c:1": true}

	web := groupHealth("web", targets, up)
	if web.IsHealthy || web.TotalURLs != 2 || web.HealthyURLs != 1 || web.UnhealthyURLs != 1 {
		t.Errorf("web = %+v, want 1/2 healthy, unhealthy group", web)
	}
	if db := groupHealth("db", targets, up); !db.IsHealthy || db.TotalURLs != 1 {
		t.Errorf("db = %+v, want healthy 1/1", db)
	}
	if none := groupHealth("none", targets, up); none.IsHealthy || none.TotalURLs != 0 {
		t.Errorf("empty group = %+v, want unhealthy with no URLs", none)
	}
	if g := groupNames(targets); strings.Join(g, ",") != "web,db" {
		t.Errorf("groupNames = %v, want [web db]", g)
	}

	results := []SearchResult{{State: "Success", Group: "web"}, {State: "Failed", Group: "web"}, {State: "Success", Group: "db"}, {State: "Success"}}
	n := nestedResult(targets, results, up)
	if n.Summary.TotalGroups != 2 || n.Summary.HealthyGroups != 1 || n.Summary.TotalURLs != 4 || n.Summary.HealthyURLs != 3 || len(n.UngroupedURLs) != 1 {
		t.Errorf("nested summary = %+v, ungrouped %d", n.Summary, len(n.UngroupedURLs))
	}
}

func TestRunExitCodes(t *testing.T) {
	open, closed := listen(t)
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"--url", open}, exitOK},
		{[]string{"--url", open + "," + closed, "--retry-count", "0"}, exitDown},
		{[]string{"--url", "ftp://x"}, exitConfig},
		{[]string{"--timeout", "soon", "--url", open}, exitConfig},
		{[]string{}, exitConfig},
		{[]string{"--version"}, exitOK},
		{[]string{"--url", open, "--circuit-threshold", "3"}, exitOK},
	} {
		var stdout, stderr bytes.Buffer
		if got := run(context.Background(), c.args, &stdout, &stderr); got != c.want {
			t.Errorf("run(%v) = %d, want %d\nstderr: %s", c.args, got, c.want, stderr.String())
		}
	}
}

func TestRunDeprecationWarning(t *testing.T) {
	open, _ := listen(t)
	var stdout, stderr bytes.Buffer
	run(context.Background(), []string{"--url", open, "--circuit-timeout", "1m"}, &stdout, &stderr)
	if !strings.Contains(stderr.String(), "--circuit-timeout") || !strings.Contains(stderr.String(), "deprecated") {
		t.Errorf("no deprecation warning for --circuit-timeout:\n%s", stderr.String())
	}
}

// TestRunConfigFileNotOverriddenByDefaults: a flag's default must not replace
// the config file's value; only a flag given on the command line does.
func TestRunConfigFileNotOverriddenByDefaults(t *testing.T) {
	open, closed := listen(t)
	_, openPort, _ := net.SplitHostPort(open)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(cfg, []byte("urls: [\"127.0.0.1\"]\nport: \""+openPort+"\"\nretry_count: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if got := run(context.Background(), []string{"--config", cfg}, &stdout, &stderr); got != exitOK {
		t.Fatalf("port from the config file was not used: exit %d\n%s%s", got, stdout.String(), stderr.String())
	}
	_, closedPort, _ := net.SplitHostPort(closed)
	if got := run(context.Background(), []string{"--config", cfg, "--port", closedPort}, &stdout, &stderr); got != exitDown {
		t.Fatalf("--port on the command line did not override the file: exit %d", got)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(` a:1, ,icmp://h {x="1", y="a,b"} ,https://e/ body="a, b"`)
	want := []string{"a:1", `icmp://h {x="1", y="a,b"}`, `https://e/ body="a, b"`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitList = %q, want %q", got, want)
	}
}
