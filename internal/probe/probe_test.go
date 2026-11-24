package probe

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// SkipWithoutICMP skips when the process may open neither a raw nor an
// unprivileged ICMP socket (no CAP_NET_RAW and GID outside ping_group_range).
func skipWithoutICMP(t *testing.T) {
	t.Helper()
	p, err := NewPinger(net.ParseIP("127.0.0.1"), PingOptions{})
	if err != nil {
		t.Skipf("no ICMP socket here: %v", err)
	}
	_ = p.Close()
}

func TestPingOnce(t *testing.T) {
	skipWithoutICMP(t)
	ctx := context.Background()
	if _, err := PingOnce(ctx, "127.0.0.1", PingOptions{}, time.Second); err != nil {
		t.Fatalf("ping 127.0.0.1: %v", err)
	}
	if _, err := PingOnce(ctx, "127.0.0.1", PingOptions{Size: 1400}, time.Second); err != nil {
		t.Fatalf("ping 127.0.0.1 with 1400 bytes: %v", err)
	}
	// 192.0.2.0/24 is TEST-NET-1: nothing answers there.
	if _, err := PingOnce(ctx, "192.0.2.1", PingOptions{}, 200*time.Millisecond); err == nil {
		t.Fatal("ping 192.0.2.1 succeeded, want a timeout")
	}
}

func TestPingOnceIPv6(t *testing.T) {
	p, err := NewPinger(net.ParseIP("::1"), PingOptions{})
	if err != nil {
		t.Skipf("no ICMPv6 socket here: %v", err)
	}
	_ = p.Close()
	if _, err := PingOnce(context.Background(), "::1", PingOptions{}, time.Second); err != nil {
		t.Skipf("::1 does not answer here (no IPv6 loopback?): %v", err)
	}
	if _, err := PingOnce(context.Background(), "::1", PingOptions{Size: 1000}, time.Second); err != nil {
		t.Fatalf("ping ::1 with 1000 bytes: %v", err)
	}
}

// TestPingDF: a don't-fragment echo that fits still gets a reply; off Linux,
// df=true is an error. icmp_linux_test.go checks the socket option itself.
func TestPingDF(t *testing.T) {
	skipWithoutICMP(t)
	_, err := PingOnce(context.Background(), "127.0.0.1", PingOptions{DF: true, Size: 1000}, time.Second)
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Fatal("df=true off Linux: want an error")
		}
		return
	}
	if err != nil {
		t.Fatalf("df ping that fits: %v", err)
	}
}

func TestResolveIP(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct{ host, version, want string }{
		{"127.0.0.1", "", "127.0.0.1"},
		{"::1", "", "::1"},
		{"127.0.0.1", "4", "127.0.0.1"},
		{"::1", "6", "::1"},
	} {
		ip, err := ResolveIP(ctx, c.host, c.version)
		if err != nil || ip.String() != c.want {
			t.Errorf("ResolveIP(%q, %q) = %v, %v; want %s", c.host, c.version, ip, err, c.want)
		}
	}
	if ip, err := ResolveIP(ctx, "127.0.0.1", "6"); err == nil {
		t.Errorf("ResolveIP(127.0.0.1, 6) = %v, want an error", ip)
	}
}

func TestCheck(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	up := target.Target{Protocol: "tcp", Host: "127.0.0.1", Port: port}
	if _, err := Check(context.Background(), up, time.Second); err != nil {
		t.Errorf("Check(%s): %v", up.Key(), err)
	}
	_ = ln.Close()
	if _, err := Check(context.Background(), up, time.Second); err == nil {
		t.Errorf("Check(%s) after close succeeded, want connection refused", up.Key())
	}
	if _, err := Check(context.Background(), target.Target{Protocol: "sctp", Host: "x", Port: "1"}, time.Second); err == nil {
		t.Error("Check with an unknown protocol succeeded")
	}
}

// TestDialClosesConnections fails if Dial leaks the connection: the server
// then waits for EOF until its deadline.
func TestDialClosesConnections(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	closed := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer func() { _ = c.Close() }()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err = c.Read(make([]byte, 1))
		closed <- err
	}()
	if _, err := Dial(context.Background(), "tcp", ln.Addr().String(), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-closed; !errors.Is(err, io.EOF) {
		if os.IsTimeout(err) {
			t.Fatal("Dial left the connection open")
		}
		t.Fatalf("server read: %v, want EOF", err)
	}
}
