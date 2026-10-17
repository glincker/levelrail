package ingress

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func shieldFor(t *testing.T, key string, rule trafficpolicy.Shield) *shieldModule {
	t.Helper()
	m := &shieldModule{Key: key, Rule: rule}
	if err := m.Provision(caddy.Context{}); err != nil {
		t.Fatal(err)
	}
	return m
}

func shieldRequest(m *shieldModule, remote, ua, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("User-Agent", ua)
	rec := httptest.NewRecorder()
	ok := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	_ = m.ServeHTTP(rec, req, ok)
	return rec
}

func TestShieldModule_Decisions(t *testing.T) {
	until := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		rule   trafficpolicy.Shield
		remote string
		ua     string
		body   string
		status []int
		retry  bool
	}{
		{"rate limit then 429", trafficpolicy.Shield{Enabled: true, RatePerMinute: 6, Burst: 1}, "198.51.100.1:5000", "Mozilla/5.0", "", []int{204, 429}, true},
		{"allowlist bypasses", trafficpolicy.Shield{Enabled: true, RatePerMinute: 6, Burst: 1, Allow: []string{"198.51.100.0/24"}}, "198.51.100.1:5000", "Mozilla/5.0", "", []int{204, 204, 204}, false},
		{"bad bot refused", trafficpolicy.Shield{Enabled: true, BlockBots: true}, "198.51.100.2:5000", "Scrapy/2.11", "", []int{403}, false},
		{"bot allowed when off", trafficpolicy.Shield{Enabled: true, MaxConcurrent: 5}, "198.51.100.2:5000", "Scrapy/2.11", "", []int{204}, false},
		{"body too large", trafficpolicy.Shield{Enabled: true, MaxBodyBytes: 4}, "198.51.100.3:5000", "Mozilla/5.0", "0123456789", []int{413}, false},
		{"under attack refuses bots without config", trafficpolicy.Shield{UnderAttackUntil: &until}, "198.51.100.4:5000", "gptbot", "", []int{403}, false},
		{"disabled passes", trafficpolicy.Shield{RatePerMinute: 1, BlockBots: true}, "198.51.100.5:5000", "gptbot", "", []int{204, 204}, false},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := shieldFor(t, "shield-test-"+strconv.Itoa(i), tc.rule)
			for n, want := range tc.status {
				rec := shieldRequest(m, tc.remote, tc.ua, tc.body)
				if rec.Code != want {
					t.Fatalf("request %d: status %d, want %d", n, rec.Code, want)
				}
				if want == 429 && (rec.Header().Get("Retry-After") == "") != !tc.retry {
					t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
				}
			}
		})
	}
}

func TestShieldReasonNamesMatchTelemetry(t *testing.T) {
	if int(trafficpolicy.ShieldReasonCount) != telemetry.ShieldReasonCount {
		t.Fatalf("reason counts differ: %d vs %d", trafficpolicy.ShieldReasonCount, telemetry.ShieldReasonCount)
	}
	for i, name := range telemetry.ShieldReasons {
		if got := trafficpolicy.ShieldReason(i).String(); got != name {
			t.Errorf("reason %d: trafficpolicy %q, telemetry %q", i, got, name)
		}
	}
}

func TestShieldModule_CountsOnlyReasons(t *testing.T) {
	const key = "shield-count-host"
	m := shieldFor(t, key, trafficpolicy.Shield{Enabled: true, BlockBots: true})
	shieldRequest(m, "198.51.100.9:1", "Scrapy", "")
	shieldRequest(m, "198.51.100.9:1", "Scrapy", "")
	shieldRequest(m, "198.51.100.9:1", "Mozilla", "")
	s := NewRequestStats()
	s.ObserveShield(key, trafficpolicy.ShieldBadBot)
	s.SetHostOwners(map[string]string{key: "web"})
	s.Observe(key, 429, time.Millisecond, 0, 0, false)
	w := s.DrainByApp()["web"]
	if w.Shield[trafficpolicy.ShieldBadBot] != 1 {
		t.Fatalf("shield counts = %v", w.Shield)
	}
	var names []string
	for _, sm := range w.Samples("service:web", time.Now()) {
		names = append(names, sm.Metric)
	}
	if !strings.Contains(strings.Join(names, ","), telemetry.ShieldMetric(int(trafficpolicy.ShieldBadBot))) {
		t.Fatalf("metrics %v lack the shield sample", names)
	}
}
