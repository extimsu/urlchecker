// Package web serves /metrics and the health endpoints.
package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const index = `<!doctype html>
<html><head><title>urlchecker</title></head><body>
<h1>urlchecker</h1>
<ul>
<li><a href="metrics">metrics</a></li>
<li><a href="healthz">healthz</a> (process alive)</li>
<li><a href="readyz">readyz</a> (targets loaded, checks running)</li>
</ul>
<p><a href="https://github.com/extimsu/urlchecker">github.com/extimsu/urlchecker</a></p>
</body></html>
`

// Handler returns the HTTP handler. ready reports whether checks are running.
func Handler(ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, index)
	})
	return mux
}

// Serve listens on addr until ctx is done, then shuts down gracefully.
func Serve(ctx context.Context, addr string, ready func() bool, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(ready),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serving metrics", "addr", addr)

	select {
	case err := <-errc:
		return fmt.Errorf("metrics server: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
