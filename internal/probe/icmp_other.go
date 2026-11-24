//go:build !linux

package probe

import (
	"errors"
	"net"
)

func listenDF(bool) (net.PacketConn, bool, error) {
	return nil, false, errors.New("df=true is only supported on Linux")
}
