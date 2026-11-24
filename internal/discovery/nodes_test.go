package discovery

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

func TestParseSpec(t *testing.T) {
	icmp, ports, err := ParseSpec(" icmp , tcp:10250,TCP:22,tcp:22")
	if err != nil || !icmp || strings.Join(ports, ",") != "10250,22" {
		t.Errorf("ParseSpec = %v %v %v", icmp, ports, err)
	}
	for _, bad := range []string{"", "udp:53", "tcp:0", "tcp:x", "icmp,ftp"} {
		if _, _, err := ParseSpec(bad); err == nil {
			t.Errorf("ParseSpec(%q): want an error", bad)
		}
	}
}

// fakeAPI serves /api/v1/nodes in two pages and checks the bearer token.
func fakeAPI(t *testing.T, nodes *atomic.Value) NodeOptions {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		items := nodes.Load().([]map[string]any)
		page := map[string]any{"metadata": map[string]any{}}
		if r.URL.Query().Get("continue") == "" && len(items) > 1 {
			page["items"] = items[:1]
			page["metadata"] = map[string]any{"continue": "next"}
		} else if r.URL.Query().Get("continue") == "next" {
			page["items"] = items[1:]
		} else {
			page["items"] = items
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	tok := filepath.Join(dir, "token")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600)
	_ = os.WriteFile(tok, []byte("secret\n"), 0o600)
	return NodeOptions{APIServer: srv.URL, CAFile: ca, TokenFile: tok}
}

func fakeNode(name string, addrs ...string) map[string]any {
	var as []map[string]string
	for _, a := range addrs {
		typ, ip, _ := strings.Cut(a, "=")
		as = append(as, map[string]string{"type": typ, "address": ip})
	}
	return map[string]any{"metadata": map[string]any{"name": name}, "status": map[string]any{"addresses": as}}
}

func keys(ts []target.Target) string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Key()+"@"+t.Labels["target_node"])
	}
	return strings.Join(out, " ")
}

func TestNodes(t *testing.T) {
	var nodes atomic.Value
	nodes.Store([]map[string]any{
		fakeNode("w1", "Hostname=w1", "InternalIP=10.0.0.1"),
		fakeNode("w2", "InternalIP=fd00::2", "InternalIP=10.0.0.2", "ExternalIP=1.2.3.4"),
		fakeNode("w3", "InternalIP=fd00::3"),
		fakeNode("self", "InternalIP=10.0.0.9"),
		fakeNode("noip", "Hostname=noip"),
	})
	o := fakeAPI(t, &nodes)
	o.ICMP, o.TCPPorts, o.ExcludeNode = true, []string{"10250"}, "self"

	ts, err := Nodes(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	want := "icmp://10.0.0.1@w1 icmp://10.0.0.2@w2 icmp://fd00::3@w3 tcp://10.0.0.1:10250@w1 tcp://10.0.0.2:10250@w2 tcp://[fd00::3]:10250@w3"
	if got := keys(ts); got != want {
		t.Errorf("targets\n got %s\nwant %s", got, want)
	}

	o.TokenFile = filepath.Join(t.TempDir(), "missing")
	if _, err := Nodes(context.Background(), o); err == nil {
		t.Error("no token file: want an error")
	}
	bad := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(bad, []byte("wrong"), 0o600)
	o.TokenFile = bad
	if _, err := Nodes(context.Background(), o); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("wrong token: err %v, want a 403", err)
	}
}

func TestWatchNodes(t *testing.T) {
	var nodes atomic.Value
	nodes.Store([]map[string]any{fakeNode("w1", "InternalIP=10.0.0.1")})
	o := fakeAPI(t, &nodes)
	o.ICMP, o.Interval = true, 20*time.Millisecond

	var mu sync.Mutex
	var calls []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		WatchNodes(ctx, o, func(ts []target.Target) { mu.Lock(); calls = append(calls, keys(ts)); mu.Unlock() }, slog.New(slog.DiscardHandler))
		close(done)
	}()
	waitFor := func(n int) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			got := len(calls)
			mu.Unlock()
			if got >= n {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("waited for %d updates, got %d", n, got)
			}
		}
	}
	waitFor(1)
	time.Sleep(80 * time.Millisecond) // several polls, no change
	nodes.Store([]map[string]any{fakeNode("w1", "InternalIP=10.0.0.1"), fakeNode("w2", "InternalIP=10.0.0.2")})
	waitFor(2)
	cancel()
	<-done
	if len(calls) != 2 || calls[1] != "icmp://10.0.0.1@w1 icmp://10.0.0.2@w2" {
		t.Errorf("updates %q, want two: the first list and the one with w2", calls)
	}
}
