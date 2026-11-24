package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWatchTargets(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "targets.txt")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a:1\n")

	var mu sync.Mutex
	var applied [][]target.Target
	load := func() ([]target.Target, error) { return target.ReadFile(file, "tcp", "80", "") }
	apply := func(ts []target.Target) { mu.Lock(); applied = append(applied, ts); mu.Unlock() }
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(applied) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchTargets(ctx, []string{file}, 20*time.Millisecond, load, apply, slog.New(slog.DiscardHandler))
		close(done)
	}()
	defer func() { cancel(); <-done }()
	wait := func(n int) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); count() < n; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("waited for %d reloads, got %d", n, count())
			}
		}
	}

	time.Sleep(60 * time.Millisecond)
	if count() != 0 {
		t.Fatal("reloaded without a change")
	}
	write("a:1\nb:2\n")
	wait(1)
	if got := applied[0]; len(got) != 2 {
		t.Errorf("after the edit: %d targets, want 2", len(got))
	}

	// A broken file keeps the current targets and reports the failure.
	write("ftp://nope\n")
	for deadline := time.Now().Add(3 * time.Second); testutil.ToFloat64(metrics.ReloadSuccess) != 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("reload_successful did not drop to 0 for a broken file")
		}
	}
	if count() != 1 {
		t.Error("a broken file was applied")
	}

	// SIGHUP reloads even without a change.
	write("c:3\n")
	wait(2)
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	wait(3)
	if testutil.ToFloat64(metrics.ReloadSuccess) != 1 {
		t.Error("reload_successful is not 1 after a good reload")
	}
}

func TestRunRejectsDuplicateTargets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run(context.Background(), []string{"--url", "icmp://h size=1,icmp://h size=2"}, &stdout, &stderr); got != exitConfig {
		t.Errorf("exit %d for a duplicate target, want %d", got, exitConfig)
	}
}

func TestSourcesMerge(t *testing.T) {
	var got []target.Target
	s := &sources{apply: func(ts []target.Target) { got = ts }}
	a, _ := target.Parse("icmp://10.0.0.1", "tcp", "80")
	b, _ := target.Parse(`icmp://10.0.0.2 {target_node="w2"}`, "tcp", "80")
	dupOfA, _ := target.Parse(`icmp://10.0.0.1 {target_node="w1"}`, "tcp", "80")
	s.setStatic([]target.Target{a})
	s.setDiscovered([]target.Target{dupOfA, b})
	if len(got) != 2 || got[0].Labels != nil || got[1].Key() != "icmp://10.0.0.2" {
		t.Errorf("merged %+v: want the static 10.0.0.1 and the discovered 10.0.0.2", got)
	}
	s.setStatic(nil)
	if len(got) != 2 || got[0].Labels["target_node"] != "w1" {
		t.Errorf("after the static target left: %+v, want both discovered", got)
	}
}
