package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
)

// DNSInfo is what a DNS check got back.
type DNSInfo struct {
	Answers []string
}

// DNS looks up t.Name with type t.RecordType, from t's resolver or the system
// one. It fails on an error, no answers, or no answer matching t.Expect.
func DNS(ctx context.Context, t target.Target, timeout time.Duration) (Result, error) {
	info := &DNSInfo{}
	res := Result{DNS: info}
	r := net.DefaultResolver
	if server := t.Resolver(); server != "" {
		d := net.Dialer{Timeout: timeout}
		r = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return d.DialContext(ctx, network+t.IPVersion, server) // udp first, tcp on truncation
			},
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	answers, err := lookup(ctx, r, t.RecordType, t.Name)
	res.Duration = time.Since(start)
	info.Answers = answers
	if err != nil {
		return res, err
	}
	if len(answers) == 0 {
		return res, fmt.Errorf("no %s records for %s", t.RecordType, t.Name)
	}
	if t.Expect != "" {
		re, err := regexp.Compile(t.Expect)
		if err != nil {
			return res, err
		}
		for _, a := range answers {
			if re.MatchString(a) {
				return res, nil
			}
		}
		return res, fmt.Errorf("no %s answer matches %q: %v", t.RecordType, t.Expect, answers)
	}
	return res, nil
}

func lookup(ctx context.Context, r *net.Resolver, typ, name string) ([]string, error) {
	var out []string
	switch typ {
	case "A", "AAAA", "":
		network := map[string]string{"A": "ip4", "": "ip4", "AAAA": "ip6"}[typ]
		ips, err := r.LookupIP(ctx, network, name)
		for _, ip := range ips {
			out = append(out, ip.String())
		}
		return out, err
	case "CNAME":
		c, err := r.LookupCNAME(ctx, name)
		if c != "" {
			out = append(out, c)
		}
		return out, err
	case "MX":
		mx, err := r.LookupMX(ctx, name)
		for _, m := range mx {
			out = append(out, strconv.Itoa(int(m.Pref))+" "+m.Host)
		}
		return out, err
	case "NS":
		ns, err := r.LookupNS(ctx, name)
		for _, n := range ns {
			out = append(out, n.Host)
		}
		return out, err
	case "TXT":
		return r.LookupTXT(ctx, name)
	case "SRV":
		_, srv, err := r.LookupSRV(ctx, "", "", name)
		for _, s := range srv {
			out = append(out, fmt.Sprintf("%d %d %d %s", s.Priority, s.Weight, s.Port, s.Target))
		}
		return out, err
	case "PTR":
		return r.LookupAddr(ctx, name)
	}
	return nil, errors.New("unsupported record type " + typ)
}
