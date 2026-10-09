package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/webhooks"
)

// The alert counters are labelled by destination kind and result. There is
// no label a URL could end up in.
func TestMetrics_AlertCounters(t *testing.T) {
	prev := alertStats
	defer func() { alertStats = prev }()
	SetAlertStats(func() []webhooks.KindStats {
		return []webhooks.KindStats{
			{Kind: "rule_notify", Sent: 2},
			{Kind: "slack", Sent: 7, Failed: 1, Dropped: 3, Suppressed: 4},
		}
	})

	rec := httptest.NewRecorder()
	Config{}.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	out := rec.Body.String()
	for _, want := range []string{
		"# TYPE tamga_alerts_total counter",
		`tamga_alerts_total{kind="slack",result="sent"} 7`,
		`tamga_alerts_total{kind="slack",result="failed"} 1`,
		`tamga_alerts_total{kind="slack",result="dropped"} 3`,
		`tamga_alerts_total{kind="slack",result="suppressed"} 4`,
		`tamga_alerts_total{kind="rule_notify",result="sent"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}
