package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// maxBody is how much of a body is read to match body=.
const maxBody = 1 << 20

// HTTPInfo is what an HTTP check saw.
type HTTPInfo struct {
	StatusCode int
	Version    float64            // 1.0, 1.1, 2 or 3
	Phases     map[string]float64 // seconds: dns, connect, tls, processing, transfer
	CertExpiry time.Time          // earliest NotAfter of the server's chain; zero for http
	Redirects  int
}

// HTTP requests t.URL on a new connection, so every check measures DNS,
// connect and TLS rather than reusing a connection. It fails on a status
// outside t.Status, or a body that doesn't match t.Body.
func HTTP(ctx context.Context, t target.Target, timeout time.Duration) (Result, error) {
	info := &HTTPInfo{Phases: map[string]float64{}}
	res := Result{HTTP: info}

	// The transport calls the trace and TLS hooks from its own goroutines.
	var mu sync.Mutex
	dialer := &net.Dialer{Timeout: timeout}
	tlsConfig := &tls.Config{
		// Verification happens in VerifyConnection, after the expiry of the
		// chain is recorded: an expired certificate still reports its expiry.
		InsecureSkipVerify: true, //nolint:gosec // verified below unless insecure=true
		VerifyConnection: func(cs tls.ConnectionState) error {
			mu.Lock()
			for _, c := range cs.PeerCertificates {
				if info.CertExpiry.IsZero() || c.NotAfter.Before(info.CertExpiry) {
					info.CertExpiry = c.NotAfter
				}
			}
			mu.Unlock()
			if t.Insecure {
				return nil
			}
			return verifyChain(cs, t.Host)
		},
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, t.Network(), addr)
		},
		TLSClientConfig:        tlsConfig,
		TLSHandshakeTimeout:    timeout,
		DisableKeepAlives:      true,
		ForceAttemptHTTP2:      true,
		MaxResponseHeaderBytes: 1 << 20,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if t.NoFollow {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			info.Redirects++
			return nil
		},
	}

	var dnsStart, connStart, tlsStart, wrote, firstByte time.Time
	mark := func(at *time.Time) func() {
		return func() { mu.Lock(); *at = time.Now(); mu.Unlock() }
	}
	add := func(phase string, from *time.Time) {
		mu.Lock()
		defer mu.Unlock()
		if !from.IsZero() {
			info.Phases[phase] += time.Since(*from).Seconds()
		}
	}
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { mark(&dnsStart)() },
		DNSDone:              func(httptrace.DNSDoneInfo) { add("dns", &dnsStart) },
		ConnectStart:         func(string, string) { mark(&connStart)() },
		ConnectDone:          func(string, string, error) { add("connect", &connStart) },
		TLSHandshakeStart:    mark(&tlsStart),
		TLSHandshakeDone:     func(tls.ConnectionState, error) { add("tls", &tlsStart) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { mark(&wrote)() },
		GotFirstResponseByte: func() { add("processing", &wrote); mark(&firstByte)() },
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), t.Method, t.URL, nil)
	if err != nil {
		return res, err
	}
	req.Header.Set("User-Agent", "urlchecker")
	if t.HostHeader != "" {
		req.Host = t.HostHeader
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Duration = time.Since(start)
		return res, err
	}
	defer func() { _ = resp.Body.Close() }()

	info.StatusCode = resp.StatusCode
	info.Version = float64(resp.ProtoMajor) + float64(resp.ProtoMinor)/10

	var body []byte
	if t.Body != "" {
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody))
	} else {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	}
	add("transfer", &firstByte)
	res.Duration = time.Since(start)
	if err != nil {
		return res, fmt.Errorf("read body: %w", err)
	}

	if !StatusMatches(t.Status, resp.StatusCode) {
		return res, fmt.Errorf("status %d, want %s", resp.StatusCode, t.Status)
	}
	if t.Body != "" {
		re, err := regexp.Compile(t.Body)
		if err != nil {
			return res, err
		}
		if !re.Match(body) {
			return res, fmt.Errorf("body does not match %q (first %d bytes read)", t.Body, len(body))
		}
	}
	return res, nil
}

// verifyChain does what crypto/tls does when InsecureSkipVerify is false.
func verifyChain(cs tls.ConnectionState, host string) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("tls: server sent no certificate")
	}
	name := cs.ServerName
	if name == "" {
		name = host
	}
	opts := x509.VerifyOptions{DNSName: name, Intermediates: x509.NewCertPool()}
	for _, c := range cs.PeerCertificates[1:] {
		opts.Intermediates.AddCert(c)
	}
	if _, err := cs.PeerCertificates[0].Verify(opts); err != nil {
		return fmt.Errorf("tls: %w", err)
	}
	return nil
}

// StatusMatches reports whether code matches a spec like "200", "2xx",
// "200-299" or "200,301". An empty spec means 200-399.
func StatusMatches(spec string, code int) bool {
	if spec == "" {
		spec = "200-399"
	}
	c := strconv.Itoa(code)
	for _, part := range strings.Split(spec, ",") {
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			l, err1 := strconv.Atoi(lo)
			h, err2 := strconv.Atoi(hi)
			if err1 == nil && err2 == nil && code >= l && code <= h {
				return true
			}
			continue
		}
		if len(part) == 3 && len(c) == 3 {
			match := true
			for i := range 3 {
				if part[i] != 'x' && part[i] != c[i] {
					match = false
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
