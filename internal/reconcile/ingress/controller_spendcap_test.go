package ingress

import (
	"context"
	"errors"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
)

type spendCapStore struct {
	ServiceStore
	rows   []spendcap.Enforcement
	err    error
	period string
}

func (s *spendCapStore) ListSpendCapEnforcements(_ context.Context, period string) ([]spendcap.Enforcement, error) {
	s.period = period
	return s.rows, s.err
}

func TestApplySpendCaps(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	services := []store.DesiredService{
		{Name: "web", Domains: []string{"web.example.com", "www.example.com"}},
		{Name: "api", Domains: []string{"api.example.com"}},
		{Name: "idle", Domains: []string{"idle.example.com"}},
	}
	tests := []struct {
		name         string
		rows         []spendcap.Enforcement
		waf          map[string]store.DomainWAF
		wantMaint    []string
		wantRPS      map[string]int
		wantRetrySec int
	}{
		{"nothing enforced", nil, nil, nil, nil, 0},
		{"pause marks every domain", []spendcap.Enforcement{{App: "web", Action: spendcap.ActionPause}}, nil,
			[]string{"web.example.com", "www.example.com"}, nil, 24 * 3600},
		{"throttle sets a rate limit", []spendcap.Enforcement{{App: "api", Action: spendcap.ActionThrottle, ThrottleRPS: 7}}, nil,
			nil, map[string]int{"api.example.com": 7}, 0},
		{"throttle never loosens a stricter limit", []spendcap.Enforcement{{App: "api", Action: spendcap.ActionThrottle, ThrottleRPS: 7}},
			map[string]store.DomainWAF{"api.example.com": {Domain: "api.example.com", RateLimitRPS: 2}}, nil, map[string]int{"api.example.com": 2}, 0},
		{"throttle tightens a looser limit", []spendcap.Enforcement{{App: "api", Action: spendcap.ActionThrottle, ThrottleRPS: 7}},
			map[string]store.DomainWAF{"api.example.com": {Domain: "api.example.com", RateLimitRPS: 50, WAFEnabled: true}}, nil, map[string]int{"api.example.com": 7}, 0},
		{"unknown app ignored", []spendcap.Enforcement{{App: "ghost", Action: spendcap.ActionPause}}, nil, nil, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Controller{store: &spendCapStore{rows: tc.rows}}
			maint := map[string]bool{}
			waf := tc.waf
			if waf == nil {
				waf = map[string]store.DomainWAF{}
			}
			settings, err := c.applySpendCaps(context.Background(), services, nil, true, ingress.FallbackResolver{}, now, maint, nil, waf)
			if err != nil {
				t.Fatal(err)
			}
			if len(maint) != len(tc.wantMaint) {
				t.Errorf("maintenance = %v, want %v", maint, tc.wantMaint)
			}
			for _, h := range tc.wantMaint {
				if !maint[h] {
					t.Errorf("%s not in maintenance", h)
				}
				if got := settings[h].RetryAfterSeconds; got != tc.wantRetrySec {
					t.Errorf("retry-after = %d, want %d", got, tc.wantRetrySec)
				}
			}
			for h, want := range tc.wantRPS {
				if waf[h].RateLimitRPS != want {
					t.Errorf("%s rps = %d, want %d", h, waf[h].RateLimitRPS, want)
				}
			}
		})
	}
}

func TestApplySpendCapsKeepsOperatorMaintenancePage(t *testing.T) {
	c := &Controller{store: &spendCapStore{rows: []spendcap.Enforcement{{App: "web", Action: spendcap.ActionPause}}}}
	own := store.DomainMaintenance{Domain: "web.example.com", Title: "Back soon", RetryAfterSeconds: 60}
	settings, err := c.applySpendCaps(context.Background(), []store.DesiredService{{Name: "web", Domains: []string{"web.example.com"}}}, nil, true, ingress.FallbackResolver{},
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), map[string]bool{}, map[string]store.DomainMaintenance{"web.example.com": own}, map[string]store.DomainWAF{})
	if err != nil || settings["web.example.com"].Title != "Back soon" {
		t.Errorf("operator page replaced: %+v %v", settings, err)
	}
}

func TestApplySpendCapsPeriodAndErrors(t *testing.T) {
	s := &spendCapStore{}
	c := &Controller{store: s}
	now := time.Date(2026, 11, 1, 0, 0, 1, 0, time.UTC)
	if _, err := c.applySpendCaps(context.Background(), nil, nil, true, ingress.FallbackResolver{}, now, map[string]bool{}, nil, map[string]store.DomainWAF{}); err != nil || s.period != "2026-11" {
		t.Errorf("period = %q err=%v", s.period, err)
	}
	s.err = errors.New("db down")
	if _, err := c.applySpendCaps(context.Background(), nil, nil, true, ingress.FallbackResolver{}, now, map[string]bool{}, nil, map[string]store.DomainWAF{}); err == nil {
		t.Error("store error swallowed: a limit must not silently lift")
	}
	bare := &Controller{store: struct{ ServiceStore }{}}
	if _, err := bare.applySpendCaps(context.Background(), nil, nil, true, ingress.FallbackResolver{}, now, nil, nil, nil); err != nil {
		t.Errorf("store without caps: %v", err)
	}
}
