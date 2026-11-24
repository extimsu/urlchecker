package target

import (
	"strings"
	"testing"
	"time"
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
		if !got.Equal(c.want) || got.Addr() != c.addr {
			t.Errorf("Parse(%q) = %+v (addr %q), want %+v (addr %q)", c.in, got, got.Addr(), c.want, c.addr)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "ftp://example.com", "tcp://", "a b:80", "http:///path", "https://u:p@h/", "dns://", "dns://10.0.0.53", "dns:///"} {
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

func TestParseOptionsAndLabels(t *testing.T) {
	got, err := Parse(`icmp://10.0.0.1  size=1472 df=true ip=4 timeout=2s {node="worker-1", zone = "fsn1-dc14", note="a \"b\""}`, "tcp", "80")
	if err != nil {
		t.Fatal(err)
	}
	want := Target{
		Protocol: "icmp", Host: "10.0.0.1", Size: 1472, DF: true, IPVersion: "4", Timeout: 2 * time.Second,
		Labels: map[string]string{"node": "worker-1", "zone": "fsn1-dc14", "note": `a "b"`},
	}
	if !got.Equal(want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got.Network() != "ip4" || got.MetricName() != "10.0.0.1" {
		t.Errorf("Network %q MetricName %q", got.Network(), got.MetricName())
	}
	tcp, _ := Parse("example.com:443 ip=6", "tcp", "80")
	if tcp.Network() != "tcp6" || tcp.MetricName() != "example.com:443" {
		t.Errorf("tcp ip=6: Network %q MetricName %q", tcp.Network(), tcp.MetricName())
	}
}

func TestParseOptionErrors(t *testing.T) {
	for _, in := range []string{
		"example.com:443 size=100",    // size is ICMP only
		"icmp://h size=big",           // not a number
		"icmp://h size=70000",         // too big
		"icmp://h df=maybe",           // not a bool
		"icmp://h ip=5",               // not 4 or 6
		"icmp://h timeout=-1s",        // not positive
		"icmp://h nope=1",             // unknown
		"icmp://h size=1 size=2",      // twice
		"icmp://h size",               // no value
		`icmp://h {node=worker}`,      // unquoted label value
		`icmp://h {target="x"}`,       // reserved label
		`icmp://h {__name__="x"}`,     // reserved prefix
		`icmp://h {a="1" b="2"}`,      // missing comma
		`icmp://h {a="1", a="2"}`,     // duplicate label
		`icmp://h {a="1`,              // unterminated
		`icmp://h }`,                  // unbalanced
		`icmp://h body="unterminated`, // unterminated option quote
	} {
		if got, err := Parse(in, "tcp", "80"); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, got)
		}
	}
}

func TestEqualAndUnique(t *testing.T) {
	a, _ := Parse(`icmp://h {n="1"}`, "tcp", "80")
	b, _ := Parse(`icmp://h {n="2"}`, "tcp", "80")
	if a.Equal(b) {
		t.Error("targets with different labels are Equal")
	}
	c, _ := Parse(`icmp://h {n="1"}`, "tcp", "80")
	if !a.Equal(c) {
		t.Error("identical targets are not Equal")
	}
	if err := Unique([]Target{a, b}); err == nil || !strings.Contains(err.Error(), "icmp://h") {
		t.Errorf("Unique = %v, want an error naming icmp://h", err)
	}
	if err := Unique([]Target{a}); err != nil {
		t.Errorf("Unique of one target: %v", err)
	}
}

func TestParseHTTPAndDNS(t *testing.T) {
	for _, c := range []struct {
		in, key, metric string
		want            Target
	}{
		{"https://example.com", "https://example.com/", "https://example.com/",
			Target{Protocol: "https", Host: "example.com", Port: "443", URL: "https://example.com/", Method: "GET", Status: "200-399"}},
		{"http://[2001:db8::1]:8080/health?x=1 method=head status=2xx,301 body=\"ok\" follow=false insecure=true host=api.local",
			"http://[2001:db8::1]:8080/health?x=1", "http://[2001:db8::1]:8080/health?x=1",
			Target{Protocol: "http", Host: "2001:db8::1", Port: "8080", URL: "http://[2001:db8::1]:8080/health?x=1",
				Method: "HEAD", Status: "2xx,301", Body: "ok", NoFollow: true, Insecure: true, HostHeader: "api.local"}},
		{"dns:///example.com", "dns:///example.com", "dns:///example.com",
			Target{Protocol: "dns", Name: "example.com", RecordType: "A"}},
		{"dns://10.0.0.53/example.com. type=mx expect=mail", "dns://10.0.0.53:53/example.com", "dns://10.0.0.53:53/example.com",
			Target{Protocol: "dns", Host: "10.0.0.53", Port: "53", Name: "example.com", RecordType: "MX", Expect: "mail"}},
		{"dns://[::1]:5353/x.test", "dns://[::1]:5353/x.test", "dns://[::1]:5353/x.test",
			Target{Protocol: "dns", Host: "::1", Port: "5353", Name: "x.test", RecordType: "A"}},
	} {
		got, err := Parse(c.in, "tcp", "80")
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) || got.Key() != c.key || got.MetricName() != c.metric {
			t.Errorf("Parse(%q)\n got %+v key %q metric %q\nwant %+v key %q metric %q", c.in, got, got.Key(), got.MetricName(), c.want, c.key, c.metric)
		}
	}
	for _, in := range []string{
		"https://h status=600", "https://h status=abc", "https://h body=(", "https://h method=g-t", "https://h follow=maybe",
		"dns:///h type=SOA", "dns:///h expect=(", "dns:///h size=1", "icmp://h status=200",
	} {
		if got, err := Parse(in, "tcp", "80"); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", in, got)
		}
	}
}
