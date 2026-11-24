package metrics

import (
	"testing"
	"time"

	"github.com/extimsu/urlchecker/internal/probe"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordAndDeleteDetails(t *testing.T) {
	ResetMetrics()
	const u = "https://example.com/"
	expiry := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	RecordCheck(u, "https", true, 0.1)
	RecordDetails(u, probe.Result{HTTP: &probe.HTTPInfo{
		StatusCode: 204, Version: 2, Redirects: 1, CertExpiry: expiry,
		Phases: map[string]float64{"dns": 0.01, "connect": 0.02},
	}})
	RecordDetails("dns:///example.com", probe.Result{DNS: &probe.DNSInfo{Answers: []string{"a", "b"}}})

	if v := testutil.ToFloat64(HTTPStatusCode.WithLabelValues(u)); v != 204 {
		t.Errorf("status %v, want 204", v)
	}
	if v := testutil.ToFloat64(TLSCertExpiry.WithLabelValues(u)); v != float64(expiry.Unix()) {
		t.Errorf("cert expiry %v, want %d", v, expiry.Unix())
	}
	if n := testutil.CollectAndCount(HTTPPhase); n != 5 {
		t.Errorf("%d phase series, want all 5", n)
	}
	if v := testutil.ToFloat64(DNSAnswers.WithLabelValues("dns:///example.com")); v != 2 {
		t.Errorf("dns answers %v, want 2", v)
	}

	DeleteCheck(u, "https")
	for name, n := range map[string]int{
		"status": testutil.CollectAndCount(HTTPStatusCode), "phase": testutil.CollectAndCount(HTTPPhase),
		"expiry": testutil.CollectAndCount(TLSCertExpiry), "current_status": testutil.CollectAndCount(CurrentStatusGauge),
	} {
		if n != 0 {
			t.Errorf("%s: %d series left after DeleteCheck", name, n)
		}
	}
	if n := testutil.CollectAndCount(DNSAnswers); n != 1 {
		t.Errorf("DeleteCheck removed another target's series: %d dns series, want 1", n)
	}
}
