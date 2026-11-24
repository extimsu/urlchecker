package main

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
)

// watchTargets reloads the targets on SIGHUP, and when the content of a
// watched file changes (checked every interval; 0 turns polling off). Polling
// the content, rather than file events, also catches a Kubernetes ConfigMap
// update, which swaps a symlink. A load that fails keeps the current targets.
func watchTargets(ctx context.Context, files []string, interval time.Duration, load func() ([]target.Target, error), apply func([]target.Target), log *slog.Logger) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	var tick <-chan time.Time
	if interval > 0 && len(files) > 0 {
		t := time.NewTicker(interval)
		defer t.Stop()
		tick = t.C
	}
	last := hashFiles(files)
	for {
		var reason string
		select {
		case <-ctx.Done():
			return
		case <-hup:
			reason = "SIGHUP"
		case <-tick:
			h := hashFiles(files)
			if h == last {
				continue
			}
			reason = "file changed"
		}
		last = hashFiles(files)
		ts, err := load()
		if err != nil {
			metrics.ReloadSuccess.Set(0)
			log.Error("reload failed, keeping the current targets", "reason", reason, "error", err)
			continue
		}
		apply(ts)
		metrics.ReloadSuccess.Set(1)
		metrics.ReloadSuccessTime.SetToCurrentTime()
		log.Info("targets reloaded", "reason", reason, "targets", len(ts))
	}
}

// hashFiles hashes the content of files; a missing file hashes as empty.
func hashFiles(files []string) [32]byte {
	h := sha256.New()
	for _, f := range files {
		b, _ := os.ReadFile(f)
		h.Write([]byte(f))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// targetInfos describes targets for urlchecker_target_info.
func targetInfos(ts []target.Target) []metrics.TargetInfo {
	infos := make([]metrics.TargetInfo, len(ts))
	for i, t := range ts {
		infos[i] = metrics.TargetInfo{Protocol: t.Protocol, Target: t.MetricName(), Labels: t.Labels}
	}
	return infos
}
