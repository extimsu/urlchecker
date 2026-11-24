package probe

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// SkipWithoutICMP skips when the process may open neither a raw nor an
// unprivileged ICMP socket (no CAP_NET_RAW and GID outside ping_group_range).
func skipWithoutICMP(t *testing.T) {
	t.Helper()
	p, err := NewPinger("127.0.0.1")
	if err != nil {
		t.Skipf("no ICMP socket here: %v", err)
	}
	_ = p.Close()
}

func TestPingOnce(t *testing.T) {
	skipWithoutICMP(t)
	if _, err := PingOnce("127.0.0.1", time.Second); err != nil {
		t.Fatalf("ping 127.0.0.1: %v", err)
	}
	// 192.0.2.0/24 is TEST-NET-1: nothing answers there.
	if _, err := PingOnce("192.0.2.1", 200*time.Millisecond); err == nil {
		t.Fatal("ping 192.0.2.1 succeeded, want a timeout")
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
