package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHandler(t *testing.T) {
	var ready atomic.Bool
	srv := httptest.NewServer(Handler(ready.Load))
	defer srv.Close()

	get := func(path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		return resp.StatusCode, b.String()
	}

	if code, _ := get("/healthz"); code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", code)
	}
	if code, _ := get("/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("/readyz before ready = %d, want 503", code)
	}
	ready.Store(true)
	if code, _ := get("/readyz"); code != http.StatusOK {
		t.Errorf("/readyz when ready = %d, want 200", code)
	}
	if code, body := get("/metrics"); code != http.StatusOK || !strings.Contains(body, "go_goroutines") {
		t.Errorf("/metrics = %d, no go_goroutines", code)
	}
	if code, body := get("/"); code != http.StatusOK || !strings.Contains(body, `href="metrics"`) {
		t.Errorf("/ = %d without a metrics link", code)
	}
	if code, _ := get("/nope"); code != http.StatusNotFound {
		t.Errorf("/nope = %d, want 404", code)
	}
}
