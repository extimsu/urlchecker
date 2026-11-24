package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

func parse(t *testing.T, line string) target.Target {
	t.Helper()
	tg, err := target.Parse(line, "tcp", "80")
	if err != nil {
		t.Fatal(err)
	}
	return tg
}

func TestHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("/fail", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", http.StatusServiceUnavailable) })
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusFound) })
	mux.HandleFunc("/host", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(r.Host)) })
	mux.HandleFunc("/method", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(r.Method)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	for _, c := range []struct {
		line   string
		ok     bool
		status int
	}{
		{srv.URL + "/ok", true, 200},
		{srv.URL + `/ok body="\"status\":\s*\"ok\""`, true, 200},
		{srv.URL + `/ok body=down`, false, 200},
		{srv.URL + "/fail", false, 503},
		{srv.URL + "/fail status=503", true, 503},
		{srv.URL + "/fail status=5xx", true, 503},
		{srv.URL + "/moved", true, 200},
		{srv.URL + "/moved follow=false", true, 302},
		{srv.URL + "/moved follow=false status=200", false, 302},
		{srv.URL + "/host host=api.local body=^api.local$", true, 200},
		{srv.URL + "/method method=post body=^POST$", true, 200},
	} {
		res, err := HTTP(ctx, parse(t, c.line), time.Second)
		if (err == nil) != c.ok || res.HTTP == nil || res.HTTP.StatusCode != c.status {
			t.Errorf("%s: err %v, status %v; want ok=%v status %d", c.line, err, res.HTTP, c.ok, c.status)
		}
	}

	res, _ := HTTP(ctx, parse(t, srv.URL+"/moved"), time.Second)
	if res.HTTP.Redirects != 1 || res.HTTP.Version != 1.1 || res.HTTP.Phases["connect"] <= 0 || res.Duration <= 0 {
		t.Errorf("redirects %d version %v phases %v duration %v", res.HTTP.Redirects, res.HTTP.Version, res.HTTP.Phases, res.Duration)
	}

	srv.Close()
	if _, err := HTTP(ctx, parse(t, srv.URL+"/ok"), time.Second); err == nil {
		t.Error("a closed server: want an error")
	}
}

func TestHTTPS(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	ctx := context.Background()

	res, err := HTTP(ctx, parse(t, srv.URL), time.Second)
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("self-signed certificate: err %v, want a certificate error", err)
	}
	// The expiry is known even when verification fails (e.g. an expired certificate).
	if want := srv.Certificate().NotAfter; !res.HTTP.CertExpiry.Equal(want) {
		t.Errorf("cert expiry after a failed verification = %v, want %v", res.HTTP.CertExpiry, want)
	}
	res, err = HTTP(ctx, parse(t, srv.URL+" insecure=true"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.Certificate().NotAfter; !res.HTTP.CertExpiry.Equal(want) {
		t.Errorf("cert expiry %v, want %v", res.HTTP.CertExpiry, want)
	}
	if res.HTTP.Version != 2 || res.HTTP.Phases["tls"] <= 0 {
		t.Errorf("version %v, tls phase %v; want HTTP/2 and a TLS time", res.HTTP.Version, res.HTTP.Phases["tls"])
	}
}

func TestStatusMatches(t *testing.T) {
	for _, c := range []struct {
		spec string
		code int
		want bool
	}{
		{"", 200, true}, {"", 302, true}, {"", 404, false},
		{"200", 200, true}, {"200", 201, false},
		{"2xx", 204, true}, {"2xx", 302, false}, {"5x3", 503, true},
		{"200-299", 299, true}, {"200-299", 300, false},
		{"200,301", 301, true}, {"200,301", 302, false},
	} {
		if got := StatusMatches(c.spec, c.code); got != c.want {
			t.Errorf("StatusMatches(%q, %d) = %v, want %v", c.spec, c.code, got, c.want)
		}
	}
}
