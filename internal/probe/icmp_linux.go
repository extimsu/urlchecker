package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// listenDF opens an ICMP socket that sets don't-fragment on every packet, so
// an echo larger than the path MTU fails instead of being fragmented. It
// opens the socket itself because x/net/icmp gives no access to it.
func listenDF(v6 bool) (net.PacketConn, bool, error) {
	level, opt, val := unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO
	rawNet, laddr, family, proto := "ip4:icmp", "0.0.0.0", unix.AF_INET, unix.IPPROTO_ICMP
	if v6 {
		level, opt, val = unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IPV6_PMTUDISC_DO
		rawNet, laddr, family, proto = "ip6:ipv6-icmp", "::", unix.AF_INET6, unix.IPPROTO_ICMPV6
	}

	var lc net.ListenConfig
	if c, rawErr := lc.ListenPacket(context.Background(), rawNet, laddr); rawErr == nil {
		var serr error
		rc, err := c.(syscall.Conn).SyscallConn()
		if err == nil {
			err = rc.Control(func(fd uintptr) { serr = unix.SetsockoptInt(int(fd), level, opt, val) })
		}
		if err == nil {
			err = serr
		}
		if err != nil {
			_ = c.Close()
			return nil, false, fmt.Errorf("set don't-fragment: %w", err)
		}
		return c, true, nil
	}

	fd, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, proto)
	if err != nil {
		return nil, false, fmt.Errorf("open ICMP socket: no CAP_NET_RAW, and unprivileged: %w", err)
	}
	if err := unix.SetsockoptInt(fd, level, opt, val); err != nil {
		_ = unix.Close(fd)
		return nil, false, fmt.Errorf("set don't-fragment: %w", err)
	}
	f := os.NewFile(uintptr(fd), "icmp")
	defer func() { _ = f.Close() }()
	c, err := net.FilePacketConn(f)
	if err != nil {
		return nil, false, err
	}
	return c, false, nil
}
