package probe

import (
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Pinger sends ICMPv4 echo requests to one host.
type Pinger struct {
	conn *icmp.PacketConn
	raw  bool
	dst  *net.IPAddr
	id   int
	seq  int
}

// NewPinger resolves host and opens an ICMP socket for it. It tries a raw
// socket first, which needs CAP_NET_RAW. Then it tries the unprivileged
// datagram socket, which needs the process GID inside net.ipv4.ping_group_range.
func NewPinger(host string) (*Pinger, error) {
	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, err
	}
	id := rand.IntN(1 << 16)
	conn, rawErr := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if rawErr == nil {
		return &Pinger{conn: conn, raw: true, dst: dst, id: id}, nil
	}
	conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("open ICMP socket: raw: %w; unprivileged: %w", rawErr, err)
	}
	// Linux replaces the ID with the socket's port; other systems keep it.
	return &Pinger{conn: conn, dst: dst, id: id}, nil
}

// IP is the resolved address being pinged.
func (p *Pinger) IP() net.IP { return p.dst.IP }

// Close closes the socket.
func (p *Pinger) Close() error { return p.conn.Close() }

// Ping sends one echo request and waits up to timeout for the matching reply.
func (p *Pinger) Ping(timeout time.Duration) (time.Duration, error) {
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

func (p *Pinger) fromTarget(peer net.Addr) bool {
	switch a := peer.(type) {
	case *net.IPAddr:
		return a.IP.Equal(p.dst.IP)
	case *net.UDPAddr:
		return a.IP.Equal(p.dst.IP)
	}
	return false
}

// PingOnce opens a socket, sends one echo request and closes the socket.
func PingOnce(host string, timeout time.Duration) (time.Duration, error) {
	p, err := NewPinger(host)
	if err != nil {
		return 0, err
	}
	defer func() { _ = p.Close() }()
	return p.Ping(timeout)
}
