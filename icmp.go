package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/extimsu/urlchecker/metrics"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// splitScheme reads an optional protocol prefix from a target:
// "icmp://host", "tcp://host:port" or "udp://host:port".
// A target without a prefix uses def.
func splitScheme(target, def string) (protocol, rest string) {
	if p, r, ok := strings.Cut(target, "://"); ok {
		return strings.ToLower(p), r
	}
	return def, target
}

// pinger sends ICMPv4 echo requests to one host.
type pinger struct {
	conn *icmp.PacketConn
	raw  bool
	dst  *net.IPAddr
	id   int
	seq  int
}

// newPinger resolves host and opens an ICMP socket for it. It tries a raw
// socket first, which needs CAP_NET_RAW. Then it tries the unprivileged
// datagram socket, which needs the process GID inside net.ipv4.ping_group_range.
// ponytail: IPv4 only and one socket per target; add ip6 and a shared socket
// when IPv6 targets or hundreds of targets are needed.
func newPinger(host string) (*pinger, error) {
	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, err
	}
	conn, rawErr := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if rawErr == nil {
		return &pinger{conn: conn, raw: true, dst: dst, id: rand.Intn(1 << 16)}, nil
	}
	conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("open ICMP socket: raw: %v; unprivileged: %v", rawErr, err)
	}
	// Linux replaces the ID with the socket's port; other systems keep it.
	return &pinger{conn: conn, dst: dst, id: rand.Intn(1 << 16)}, nil
}

func (p *pinger) close() { p.conn.Close() }

// ping sends one echo request and waits up to timeout for the matching reply.
func (p *pinger) ping(timeout time.Duration) (time.Duration, error) {
	p.seq = (p.seq + 1) & 0xffff
	req, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: &icmp.Echo{ID: p.id, Seq: p.seq, Data: []byte("urlchecker")},
	}).Marshal(nil)
	if err != nil {
		return 0, err
	}

	var dst net.Addr = p.dst
	if !p.raw {
		dst = &net.UDPAddr{IP: p.dst.IP}
	}
	start := time.Now()
	if err := p.conn.SetReadDeadline(start.Add(timeout)); err != nil {
		return 0, err
	}
	if _, err := p.conn.WriteTo(req, dst); err != nil {
		return 0, err
	}

	buf := make([]byte, 1500)
	for {
		n, peer, err := p.conn.ReadFrom(buf)
		if err != nil {
			return 0, err
		}
		rtt := time.Since(start)
		if !p.fromTarget(peer) {
			continue
		}
		m, err := icmp.ParseMessage(1, buf[:n]) // 1 = ICMPv4
		if err != nil || m.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		e, ok := m.Body.(*icmp.Echo)
		// A raw socket sees every ICMP packet on the host, so match our ID.
		// The kernel sets and filters the ID on the unprivileged socket.
		if !ok || e.Seq != p.seq || (p.raw && e.ID != p.id) {
			continue
		}
		return rtt, nil
	}
}

func (p *pinger) fromTarget(peer net.Addr) bool {
	switch a := peer.(type) {
	case *net.IPAddr:
		return a.IP.Equal(p.dst.IP)
	case *net.UDPAddr:
		return a.IP.Equal(p.dst.IP)
	}
	return false
}

// pingOnce is the one-shot ICMP check used by probe.
func pingOnce(addr string, timeout time.Duration) error {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	p, err := newPinger(host)
	if err != nil {
		return err
	}
	defer p.close()
	_, err = p.ping(timeout)
	return err
}

// runPinger pings host every interval until stop is closed. It exports the
// same counters as smokeping_prober, so packet loss is
// 1 - rate(urlchecker_icmp_response_duration_seconds_count) / rate(urlchecker_icmp_requests_total).
func runPinger(host string, interval, timeout time.Duration, stop <-chan struct{}) {
	if timeout > interval {
		timeout = interval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var p *pinger
	up := true // log only when the result changes
	for {
		if p == nil {
			var err error
			if p, err = newPinger(host); err != nil {
				log.Printf("ICMP %s: %v", host, err)
			} else {
				// Create the series now, so a target that never answers shows 100% loss, not no data.
				metrics.ICMPResponseDuration.WithLabelValues(host, p.dst.IP.String())
				defer p.close()
			}
		}
		if p != nil {
			ip := p.dst.IP.String()
			metrics.ICMPRequests.WithLabelValues(host, ip).Inc()
			rtt, err := p.ping(timeout)
			if err == nil {
				metrics.ICMPResponseDuration.WithLabelValues(host, ip).Observe(rtt.Seconds())
			}
			if (err == nil) != up {
				up = err == nil
				if up {
					log.Printf("ICMP %s (%s): replies again", host, ip)
				} else {
					log.Printf("ICMP %s (%s): no reply: %v", host, ip, err)
				}
			}
		}
		select {
		case <-ticker.C:
		case <-stop:
			return
		}
	}
}
