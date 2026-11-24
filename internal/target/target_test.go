package target

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Target
		addr string
	}{
		{"example.com", Target{Protocol: "tcp", Host: "example.com", Port: "80"}, "example.com:80"},
		{"example.com:443", Target{Protocol: "tcp", Host: "example.com", Port: "443"}, "example.com:443"},
		{"TCP://example.com:443", Target{Protocol: "tcp", Host: "example.com", Port: "443"}, "example.com:443"},
		{"udp://10.0.0.53:53", Target{Protocol: "udp", Host: "10.0.0.53", Port: "53"}, "10.0.0.53:53"},
		{"icmp://10.0.0.1", Target{Protocol: "icmp", Host: "10.0.0.1"}, "10.0.0.1"},
		{"icmp://10.0.0.1:80", Target{Protocol: "icmp", Host: "10.0.0.1"}, "10.0.0.1"},
		{"[2001:db8::1]:443", Target{Protocol: "tcp", Host: "2001:db8::1", Port: "443"}, "[2001:db8::1]:443"},
		{"[2001:db8::1]", Target{Protocol: "tcp", Host: "2001:db8::1", Port: "80"}, "[2001:db8::1]:80"},
		{"icmp://2001:db8::1", Target{Protocol: "icmp", Host: "2001:db8::1"}, "2001:db8::1"},
		{"  host:22  ", Target{Protocol: "tcp", Host: "host", Port: "22"}, "host:22"},
	} {
		got, err := Parse(c.in, "tcp", "80")
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if got != c.want || got.Addr() != c.addr {
			t.Errorf("Parse(%q) = %+v (addr %q), want %+v (addr %q)", c.in, got, got.Addr(), c.want, c.addr)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "ftp://example.com", "tcp://", "a b:80", "http://x/y"} {
		if got, err := Parse(in, "tcp", "80"); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, got)
		}
	}
	if _, err := Parse("example.com", "tcp", ""); err == nil {
		t.Error("Parse without a port or default port: want an error")
	}
}

func TestRead(t *testing.T) {
	in := `# comment
example.com:443

[group:web]
web1:80
icmp://10.0.0.1
[group: db ]
db1:5432
`
	got, err := Read(strings.NewReader(in), "t", "tcp", "80", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ key, group string }{
		{"tcp://example.com:443", ""}, {"tcp://web1:80", "web"}, {"icmp://10.0.0.1", "web"}, {"tcp://db1:5432", "db"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Key() != w.key || got[i].Group != w.group {
			t.Errorf("target %d = %s in %q, want %s in %q", i, got[i].Key(), got[i].Group, w.key, w.group)
		}
	}

	got, _ = Read(strings.NewReader(in), "t", "tcp", "80", "all")
	for _, tg := range got {
		if tg.Group != "all" {
			t.Errorf("%s: group %q, want the override \"all\"", tg.Key(), tg.Group)
		}
	}

	if _, err := Read(strings.NewReader("ok:1\nftp://x\n"), "list.txt", "tcp", "80", ""); err == nil || !strings.Contains(err.Error(), "list.txt:2:") {
		t.Errorf("error = %v, want one naming list.txt:2", err)
	}
}
