// Package probe runs single checks against a target.
package probe

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// Check runs one check of t and returns how long it took. The target's own
// timeout option, when set, replaces timeout.
func Check(ctx context.Context, t target.Target, timeout time.Duration) (time.Duration, error) {
	if t.Timeout > 0 {
		timeout = t.Timeout
	}
	switch t.Protocol {
	case "tcp", "udp":
		return Dial(ctx, t.Network(), t.Addr(), timeout)
	case "icmp":
		return PingOnce(ctx, t.Host, PingOptionsOf(t), timeout)
	}
	return 0, fmt.Errorf("unsupported protocol %q", t.Protocol)
}

// Dial opens a TCP or UDP connection to addr and closes it right away.
// Every connection is closed: a leaked one costs a socket and file descriptor
// per check and leaves a connection open on the target.
//
// A UDP "connection" only proves the address resolves and a route exists;
// nothing is sent.
func Dial(ctx context.Context, network, addr string, timeout time.Duration) (time.Duration, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, network, addr)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, err
	}
	return elapsed, conn.Close()
}

// PingOptionsOf returns the ICMP options of t.
func PingOptionsOf(t target.Target) PingOptions {
	return PingOptions{IPVersion: t.IPVersion, Size: t.Size, DF: t.DF}
}
