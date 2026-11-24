package probe

import (
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// TestListenDFSetsOption reads IP_MTU_DISCOVER back from the socket: the
// kernel then sets DF on every echo and refuses ones above the path MTU.
func TestListenDFSetsOption(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		c, _, err := listenDF(v6)
		if err != nil {
			t.Logf("v6=%v: no ICMP socket here: %v", v6, err)
			continue
		}
		level, opt, want := unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO
		if v6 {
			level, opt, want = unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IPV6_PMTUDISC_DO
		}
		rc, err := c.(syscall.Conn).SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var got int
		var gerr error
		if err := rc.Control(func(fd uintptr) { got, gerr = unix.GetsockoptInt(int(fd), level, opt) }); err != nil || gerr != nil {
			t.Fatalf("v6=%v: getsockopt: %v %v", v6, err, gerr)
		}
		if got != want {
			t.Errorf("v6=%v: MTU discover = %d, want %d (DO)", v6, got, want)
		}
		_ = c.Close()
	}
}
