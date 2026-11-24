package metrics

import (
	"github.com/extimsu/urlchecker/internal/probe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPStatusCode is the status code of the last HTTP response.
	HTTPStatusCode = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_http_status_code",
		Help: "Status code of the last HTTP response (0 when there was none)",
	}, []string{"url"})
	// HTTPVersion is the protocol version of the last response.
	HTTPVersion = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_http_version",
		Help: "HTTP version of the last response, e.g. 1.1 or 2",
	}, []string{"url"})
	// HTTPPhase is how long each phase of the last request took.
	HTTPPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_http_phase_duration_seconds",
		Help: "Duration of each phase of the last HTTP request: dns, connect, tls, processing (to the first byte), transfer",
	}, []string{"url", "phase"})
	// HTTPRedirects is how many redirects the last request followed.
	HTTPRedirects = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_http_redirects",
		Help: "Redirects followed by the last HTTP request",
	}, []string{"url"})
	// TLSCertExpiry is when the first certificate of the server's chain expires.
	TLSCertExpiry = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_tls_cert_expiry_timestamp_seconds",
		Help: "Unix time when the earliest-expiring certificate the HTTPS server sent expires",
	}, []string{"url"})
	// DNSAnswers is how many answers the last lookup returned.
	DNSAnswers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "urlchecker_dns_answers",
		Help: "Number of answers the last DNS lookup returned",
	}, []string{"url"})

	detailVecs = []*prometheus.GaugeVec{HTTPStatusCode, HTTPVersion, HTTPPhase, HTTPRedirects, TLSCertExpiry, DNSAnswers}
)

var phases = []string{"dns", "connect", "tls", "processing", "transfer"}

// RecordDetails records the HTTP or DNS details of a result.
func RecordDetails(url string, r probe.Result) {
	if h := r.HTTP; h != nil {
		HTTPStatusCode.WithLabelValues(url).Set(float64(h.StatusCode))
		HTTPVersion.WithLabelValues(url).Set(h.Version)
		HTTPRedirects.WithLabelValues(url).Set(float64(h.Redirects))
		for _, p := range phases {
			HTTPPhase.WithLabelValues(url, p).Set(h.Phases[p])
		}
		if !h.CertExpiry.IsZero() {
			TLSCertExpiry.WithLabelValues(url).Set(float64(h.CertExpiry.Unix()))
		}
	}
	if d := r.DNS; d != nil {
		DNSAnswers.WithLabelValues(url).Set(float64(len(d.Answers)))
	}
}

func deleteDetails(url string) {
	for _, v := range detailVecs {
		v.DeletePartialMatch(prometheus.Labels{"url": url})
	}
}
