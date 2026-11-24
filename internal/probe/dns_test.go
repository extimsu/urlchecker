package probe

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// fakeDNS answers A and MX for example.test. and NXDOMAIN for anything else.
func fakeDNS(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var p dnsmessage.Parser
			h, err := p.Start(buf[:n])
			if err != nil {
				continue
			}
			q, err := p.Question()
			if err != nil {
				continue
			}
			known := q.Name.String() == "example.test."
			rh := dnsmessage.Header{ID: h.ID, Response: true, Authoritative: true}
			if !known {
				rh.RCode = dnsmessage.RCodeNameError
			}
			b := dnsmessage.NewBuilder(nil, rh)
			_ = b.StartQuestions()
			_ = b.Question(q)
			_ = b.StartAnswers()
			rr := dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: 60}
			if known && q.Type == dnsmessage.TypeA {
				_ = b.AResource(rr, dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
			}
			if known && q.Type == dnsmessage.TypeMX {
				_ = b.MXResource(rr, dnsmessage.MXResource{Pref: 10, MX: dnsmessage.MustNewName("mail.example.test.")})
			}
			msg, err := b.Finish()
			if err == nil {
				_, _ = pc.WriteTo(msg, addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

func TestDNS(t *testing.T) {
	server := fakeDNS(t)
	ctx := context.Background()
	for _, c := range []struct {
		line    string
		ok      bool
		answers int
	}{
		{"dns://" + server + "/example.test", true, 1},
		{"dns://" + server + "/example.test expect=^192\\.0\\.2\\.10$", true, 1},
		{"dns://" + server + "/example.test expect=^10\\.", false, 1},
		{"dns://" + server + "/example.test type=MX expect=mail.example.test", true, 1},
		{"dns://" + server + "/missing.test", false, 0},
		{"dns://" + server + "/example.test type=AAAA", false, 0},
	} {
		res, err := DNS(ctx, parse(t, c.line), time.Second)
		if (err == nil) != c.ok || res.DNS == nil || len(res.DNS.Answers) != c.answers {
			t.Errorf("%s: err %v, answers %v; want ok=%v and %d answers", c.line, err, res.DNS, c.ok, c.answers)
		}
	}
}
