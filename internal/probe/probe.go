// Package probe runs single checks against a target.
package probe

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// Check runs one check of t and returns how long it took.
func Check(ctx context.Context, t target.Target, timeout time.Duration) (time.Duration, error) {
	switch t.Protocol {
	case "tcp", "udp":
		return Dial(ctx, t.Protocol, t.Addr(), timeout)
	case "icmp":
		return PingOnce(t.Host, timeout)
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
