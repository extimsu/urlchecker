// Package discovery finds targets in Kubernetes. It talks to the API server
// with the pod's service-account token and the standard library, so the
// binary doesn't carry client-go for one list call.
package discovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/extimsu/urlchecker/internal/target"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// In-cluster service-account files.
const (
	saDir     = "/var/run/secrets/kubernetes.io/serviceaccount"
	pageLimit = 500
)

var (
	discoveredTargets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "urlchecker_discovery_targets",
		Help: "Targets found by Kubernetes node discovery",
	})
	discoveryErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "urlchecker_discovery_errors_total",
		Help: "Failed Kubernetes node discovery runs",
	})
)

// NodeOptions configure node discovery.
type NodeOptions struct {
	ICMP     bool     // ping every node's InternalIP
	TCPPorts []string // and connect to these ports on it
	// ExcludeNode is skipped, usually the node the pod runs on (NODE_NAME).
	ExcludeNode string
	Interval    time.Duration

	// API access; empty values mean in-cluster defaults.
	APIServer string // https://host:port; default from KUBERNETES_SERVICE_HOST/PORT
	TokenFile string
	CAFile    string
}

// ParseSpec reads a --discover-nodes value: "icmp", "tcp:10250", or both
// separated by commas.
func ParseSpec(spec string) (icmp bool, ports []string, err error) {
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		switch {
		case part == "":
		case part == "icmp":
			icmp = true
		case strings.HasPrefix(part, "tcp:"):
			p := strings.TrimPrefix(part, "tcp:")
			if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
				return false, nil, fmt.Errorf("discover nodes: invalid port in %q", part)
			}
			if !slices.Contains(ports, p) {
				ports = append(ports, p)
			}
		default:
			return false, nil, fmt.Errorf("discover nodes: %q: want icmp or tcp:<port>", part)
		}
	}
	if !icmp && len(ports) == 0 {
		return false, nil, errors.New("discover nodes: nothing to check; want icmp and/or tcp:<port>")
	}
	return icmp, ports, nil
}

// Nodes lists the nodes once and returns their targets, labelled
// {target_node="<name>"}.
func Nodes(ctx context.Context, o NodeOptions) ([]target.Target, error) {
	server, client, err := o.client()
	if err != nil {
		return nil, err
	}
	tokenFile := o.TokenFile
	if tokenFile == "" {
		tokenFile = saDir + "/token"
	}

	var targets []target.Target
	cont := ""
	for {
		// Read the token every time: projected tokens rotate.
		token, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("service-account token: %w", err)
		}
		q := url.Values{"limit": {strconv.Itoa(pageLimit)}}
		if cont != "" {
			q.Set("continue", cont)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/v1/nodes?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		var list nodeList
		err = json.NewDecoder(resp.Body).Decode(&list)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list nodes: %s (the service account needs list on nodes)", resp.Status)
		}
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		for _, n := range list.Items {
			if n.Metadata.Name == o.ExcludeNode {
				continue
			}
			ip := n.internalIP()
			if ip == "" {
				continue
			}
			labels := map[string]string{"target_node": n.Metadata.Name}
			if o.ICMP {
				targets = append(targets, target.Target{Protocol: "icmp", Host: ip, Labels: labels})
			}
			for _, p := range o.TCPPorts {
				targets = append(targets, target.Target{Protocol: "tcp", Host: ip, Port: p, Labels: labels})
			}
		}
		if cont = list.Metadata.Continue; cont == "" {
			break
		}
	}
	slices.SortFunc(targets, func(a, b target.Target) int { return strings.Compare(a.Key(), b.Key()) })
	return targets, nil
}

// WatchNodes lists the nodes every Interval and calls apply when the targets
// change. A failed list keeps the last targets.
func WatchNodes(ctx context.Context, o NodeOptions, apply func([]target.Target), log *slog.Logger) {
	var last []target.Target
	first := true
	for {
		ts, err := Nodes(ctx, o)
		switch {
		case err != nil:
			discoveryErrors.Inc()
			log.Error("node discovery failed, keeping the last targets", "error", err)
		case first || !equal(ts, last):
			apply(ts)
			discoveredTargets.Set(float64(len(ts)))
			log.Info("node discovery", "targets", len(ts))
			last, first = ts, false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(o.Interval):
		}
	}
}

func equal(a, b []target.Target) bool {
	return slices.EqualFunc(a, b, func(x, y target.Target) bool { return x.Equal(y) })
}

func (o NodeOptions) client() (string, *http.Client, error) {
	server := o.APIServer
	if server == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return "", nil, errors.New("not in a Kubernetes pod: KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT are unset")
		}
		server = "https://" + net.JoinHostPort(host, port)
	}
	caFile := o.CAFile
	if caFile == "" {
		caFile = saDir + "/ca.crt"
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return "", nil, fmt.Errorf("cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return "", nil, fmt.Errorf("cluster CA %s: no certificates", caFile)
	}
	return server, &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}, nil
}

type nodeList struct {
	Metadata struct {
		Continue string `json:"continue"`
	} `json:"metadata"`
	Items []node `json:"items"`
}

type node struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
	} `json:"status"`
}

// internalIP returns the node's InternalIP, IPv4 first on dual-stack nodes.
// ponytail: one address per node; add per-family targets if IPv6 node paths need their own probes.
func (n node) internalIP() string {
	var v6 string
	for _, a := range n.Status.Addresses {
		if a.Type != "InternalIP" {
			continue
		}
		ip := net.ParseIP(a.Address)
		switch {
		case ip == nil:
		case ip.To4() != nil:
			return a.Address
		case v6 == "":
			v6 = a.Address
		}
	}
	return v6
}
