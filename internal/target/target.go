// Package target parses check targets. A target line is the target, optional
// key=value options and an optional label block:
//
//	host | host:port | tcp://host:port | udp://host:port | icmp://host
//	icmp://10.0.0.1 size=1472 df=true {node="worker-1"}
package target

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Protocols lists the supported protocols.
var Protocols = []string{"tcp", "udp", "icmp"}

// Target is one thing to check.
type Target struct {
	Protocol string // tcp, udp or icmp
	Host     string // host name or IP, without brackets
	Port     string // empty for icmp
	Group    string // group from a [group:name] section or the config

	// Options.
	Timeout   time.Duration // timeout=; 0 means the global timeout
	IPVersion string        // ip=4 or ip=6; empty prefers IPv4
	Size      int           // size=: ICMP payload bytes; 0 means the default
	DF        bool          // df=true: ICMP with the don't-fragment bit (Linux)

	// Labels from the {name="value"} block, exported on urlchecker_target_info.
	Labels map[string]string
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

// MetricName is the value that identifies the target on its metrics: the
// url label of the check metrics, or the host label of the ICMP metrics.
func (t Target) MetricName() string {
	if t.Protocol == "icmp" {
		return t.Host
	}
	return t.Addr()
}

// Network is the network to dial or resolve: "tcp", "tcp4", "udp6", "ip"...
func (t Target) Network() string {
	base := t.Protocol
	if base == "icmp" {
		base = "ip"
	}
	return base + t.IPVersion
}

// Equal reports whether a and b are the same target with the same settings
// and labels. Parse leaves Labels nil when there are none.
func (t Target) Equal(o Target) bool { return reflect.DeepEqual(t, o) }

// option is one key=value option and the protocols it applies to.
type option struct {
	protocols []string
	set       func(t *Target, v string) error
}

var options = map[string]option{
	"timeout": {Protocols, func(t *Target, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return fmt.Errorf("want a positive duration like 2s")
		}
		t.Timeout = d
		return nil
	}},
	"ip": {Protocols, func(t *Target, v string) error {
		if v != "4" && v != "6" {
			return fmt.Errorf("want 4 or 6")
		}
		t.IPVersion = v
		return nil
	}},
	"size": {[]string{"icmp"}, func(t *Target, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 65000 {
			return fmt.Errorf("want payload bytes from 0 to 65000")
		}
		t.Size = n
		return nil
	}},
	"df": {[]string{"icmp"}, func(t *Target, v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("want true or false")
		}
		t.DF = b
		return nil
	}},
}

// OptionNames lists the options valid for a protocol, sorted.
func OptionNames(protocol string) []string {
	var names []string
	for name, o := range options {
		if slices.Contains(o.protocols, protocol) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Parse reads one target line. defProto and defPort apply when the target
// has no protocol prefix or no port.
func Parse(line, defProto, defPort string) (Target, error) {
	s, opts, labels, err := splitLine(line)
	if err != nil {
		return Target{}, err
	}
	t, err := parseTarget(s, defProto, defPort)
	if err != nil {
		return Target{}, err
	}
	seen := map[string]bool{}
	for _, kv := range opts {
		k, v := kv[0], kv[1]
		o, ok := options[k]
		if !ok || !slices.Contains(o.protocols, t.Protocol) {
			return Target{}, fmt.Errorf("target %q: unknown option %q for %s (valid: %s)", s, k, t.Protocol, strings.Join(OptionNames(t.Protocol), ", "))
		}
		if seen[k] {
			return Target{}, fmt.Errorf("target %q: option %q set twice", s, k)
		}
		seen[k] = true
		if err := o.set(&t, v); err != nil {
			return Target{}, fmt.Errorf("target %q: option %s=%q: %w", s, k, v, err)
		}
	}
	if len(labels) > 0 {
		t.Labels = labels
	}
	return t, nil
}

func parseTarget(s, defProto, defPort string) (Target, error) {
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
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

// Unique returns an error naming the first target that appears twice. Two
// copies would write the same metric series.
func Unique(targets []Target) error {
	seen := map[string]bool{}
	for _, t := range targets {
		if seen[t.Key()] {
			return fmt.Errorf("target %s is listed twice", t.Key())
		}
		seen[t.Key()] = true
	}
	return nil
}
