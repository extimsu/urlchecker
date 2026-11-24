package probe

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// PingOptions are the per-target ICMP settings.
type PingOptions struct {
	IPVersion string // "4", "6" or "" (prefer IPv4)
	Size      int    // payload bytes; 0 sends "urlchecker"
	DF        bool   // set don't-fragment (Linux only)
}

// Pinger sends ICMP echo requests to one address.
type Pinger struct {
	conn net.PacketConn
	raw  bool
	v6   bool
	dst  net.IP
	id   int
	seq  int
	data []byte
	buf  []byte
}

// ResolveIP returns host's address for the IP version ("4", "6", or ""
// for either, preferring IPv4).
func ResolveIP(ctx context.Context, host, version string) (net.IP, error) {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		var err error
		if ips, err = net.DefaultResolver.LookupIP(ctx, "ip"+version, host); err != nil {
			return nil, err
		}
	}
	for _, want4 := range []bool{true, false} {
		for _, ip := range ips {
			is4 := ip.To4() != nil
			if is4 == want4 && (version == "" || (version == "4") == is4) {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("%s has no IPv%s address", host, version)
}

// NewPinger opens an ICMP socket for ip. It tries a raw socket first, which
// needs CAP_NET_RAW, then the unprivileged datagram socket, which needs the
// process GID inside net.ipv4.ping_group_range (the same sysctl covers IPv6).
func NewPinger(ip net.IP, o PingOptions) (*Pinger, error) {
	v6 := ip.To4() == nil
	p := &Pinger{dst: ip, v6: v6, id: rand.IntN(1 << 16), data: []byte("urlchecker")}
	if o.Size > 0 {
		p.data = make([]byte, o.Size)
		for i := range p.data {
			p.data[i] = byte(i)
		}
	}
	p.buf = make([]byte, max(1500, len(p.data)+128))

	var err error
	if o.DF {
		if p.conn, p.raw, err = listenDF(v6); err != nil {
			return nil, err
		}
		return p, nil
	}
	rawNet, dgramNet, any := "ip4:icmp", "udp4", "0.0.0.0"
	if v6 {
		rawNet, dgramNet, any = "ip6:ipv6-icmp", "udp6", "::"
	}
	c, rawErr := icmp.ListenPacket(rawNet, any)
	if rawErr == nil {
		p.conn, p.raw = c, true
		return p, nil
	}
	if c, err = icmp.ListenPacket(dgramNet, any); err != nil {
		return nil, fmt.Errorf("open ICMP socket: raw: %w; unprivileged: %w", rawErr, err)
	}
	// Linux replaces the ID with the socket's port; other systems keep it.
	p.conn = c
	return p, nil
}

// IP is the address being pinged.
func (p *Pinger) IP() net.IP { return p.dst }

// Close closes the socket.
func (p *Pinger) Close() error { return p.conn.Close() }

// Ping sends one echo request and waits up to timeout for the matching reply.
func (p *Pinger) Ping(timeout time.Duration) (time.Duration, error) {
	p.seq = (p.seq + 1) & 0xffff
	msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: p.id, Seq: p.seq, Data: p.data}}
	proto, reply := 1, icmp.Type(ipv4.ICMPTypeEchoReply) // 1 = ICMPv4
	if p.v6 {
		msg.Type = ipv6.ICMPTypeEchoRequest
		proto, reply = 58, ipv6.ICMPTypeEchoReply // 58 = ICMPv6; the kernel fills in the checksum
	}
	req, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}

	var dst net.Addr = &net.IPAddr{IP: p.dst}
	if !p.raw {
		dst = &net.UDPAddr{IP: p.dst}
	}
	start := time.Now()
	if err := p.conn.SetReadDeadline(start.Add(timeout)); err != nil {
		return 0, err
	}
	if _, err := p.conn.WriteTo(req, dst); err != nil {
		return 0, err // with df=true, EMSGSIZE when the packet exceeds the interface MTU
	}
	for {
		n, peer, err := p.conn.ReadFrom(p.buf)
		if err != nil {
			return 0, err
		}
		rtt := time.Since(start)
		if !p.fromTarget(peer) {
			continue
		}
		m, err := icmp.ParseMessage(proto, p.buf[:n])
		if err != nil || m.Type != reply {
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
		return a.IP.Equal(p.dst)
	case *net.UDPAddr:
		return a.IP.Equal(p.dst)
	}
	return false
}

// PingOnce resolves host, sends one echo request and closes the socket.
func PingOnce(ctx context.Context, host string, o PingOptions, timeout time.Duration) (time.Duration, error) {
	ip, err := ResolveIP(ctx, host, o.IPVersion)
	if err != nil {
		return 0, err
	}
	p, err := NewPinger(ip, o)
	if err != nil {
		return 0, err
	}
	defer func() { _ = p.Close() }()
	return p.Ping(timeout)
}
