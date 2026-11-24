// Package target parses check targets: "host", "host:port",
// "tcp://host:port", "udp://host:port" and "icmp://host".
package target

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// Protocols lists the supported protocols.
var Protocols = []string{"tcp", "udp", "icmp"}

// Target is one thing to check.
type Target struct {
	Protocol string // tcp, udp or icmp
	Host     string // host name or IP, without brackets
	Port     string // empty for icmp
	Group    string // group from a [group:name] section or the config
}

// Addr is the address to dial: "host:port", "[v6]:port", or the host for ICMP.
// It is also the value of the url label on the check metrics.
func (t Target) Addr() string {
	if t.Port == "" {
		return t.Host
	}
	return net.JoinHostPort(t.Host, t.Port)
}

// Key identifies a target across reloads.
func (t Target) Key() string { return t.Protocol + "://" + t.Addr() }

// String is the target as written in a file, with its protocol.
func (t Target) String() string { return t.Key() }

// Parse reads one target. defProto and defPort apply when the target has no
// protocol prefix or no port.
func Parse(s, defProto, defPort string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Target{}, fmt.Errorf("empty target")
	}
	proto, rest := strings.ToLower(defProto), s
	if p, r, ok := strings.Cut(s, "://"); ok {
		proto, rest = strings.ToLower(p), r
	}
	if !supported(proto) {
		return Target{}, fmt.Errorf("target %q: unsupported protocol %q (want one of %s)", s, proto, strings.Join(Protocols, ", "))
	}

	t := Target{Protocol: proto}
	if h, p, err := net.SplitHostPort(rest); err == nil {
		t.Host, t.Port = h, p
	} else {
		t.Host, t.Port = strings.Trim(rest, "[]"), defPort
	}
	if proto == "icmp" {
		t.Port = "" // a port on an ICMP target is ignored
	}
	if t.Host == "" || strings.ContainsAny(t.Host, "/ ") {
		return Target{}, fmt.Errorf("target %q: invalid host %q", s, t.Host)
	}
	if t.Port == "" && proto != "icmp" {
		return Target{}, fmt.Errorf("target %q: no port and no default port", s)
	}
	return t, nil
}

func supported(proto string) bool {
	for _, p := range Protocols {
		if p == proto {
			return true
		}
	}
	return false
}

// ReadFile reads a target list: one target per line, "#" comments, and
// optional "[group:name]" section headers. group, when set, overrides the
// sections for every target.
func ReadFile(path, defProto, defPort, group string) ([]Target, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return Read(f, path, defProto, defPort, group)
}

// Read is ReadFile for any reader. name is used in error messages.
func Read(r io.Reader, name, defProto, defPort, group string) ([]Target, error) {
	var targets []Target
	section := ""
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if _, g, ok := strings.Cut(strings.Trim(line, "[]"), ":"); ok {
				section = strings.TrimSpace(g)
				continue
			}
		}
		t, err := Parse(line, defProto, defPort)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, n, err)
		}
		t.Group = section
		if group != "" {
			t.Group = group
		}
		targets = append(targets, t)
	}
	return targets, sc.Err()
}
