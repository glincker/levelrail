package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeTraffic answers by whether the range ends at the evaluation instant (current) or a day before (baseline).
type fakeTraffic struct {
	now       time.Time
	cur, base int
	missing   bool
}

func (f *fakeTraffic) DomainRequests(_ context.Context, _, _ string, _, until time.Time) (int, bool) {
	if f.missing {
		return 0, false
	}
	if until.Equal(f.now) {
		return f.cur, true
	}
	return f.base, true
}

type fakeParked struct{ domains []string }

func (f fakeParked) ListParkedDomains(context.Context) ([]store.ParkedDomain, error) {
	out := make([]store.ParkedDomain, 0, len(f.domains))
	for _, d := range f.domains {
		out = append(out, store.ParkedDomain{Domain: d})
	}
	return out, nil
}

func TestEvaluateDomainTrafficDrop(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	list := fakeDomainList{domains: []store.ServiceDomain{{Domain: "shop.example.com", ServiceName: "web"}}}
	traffic := &fakeTraffic{now: now, cur: 100, base: 100}
	r := Rule{ID: "td", Kind: KindDomainTrafficDrop, ResourceID: "service:web"}
	e := newTrafficTestEngine(newFakeRuleStore(r), nil, nil, &spyNotifier{})
	e.SetTrafficSources(list, nil)
	e.SetDomainTraffic(traffic)
	eval := func(prev Rule) (Rule, []string) {
		t.Helper()
		next, n, err := e.EvaluateDomainTrafficDrop(context.Background(), prev, now)
		if err != nil {
			t.Fatal(err)
		}
		return next, n
	}

	if next, _ := eval(r); next.Firing {
		t.Fatal("fired with steady traffic")
	}
	traffic.cur = 40
	fired, notices := eval(r)
	if !fired.Firing || len(notices) != 1 {
		t.Fatalf("drop to 40%% should fire: firing=%v notices=%v", fired.Firing, notices)
	}
	traffic.base = 19
	if next, _ := eval(r); next.Firing {
		t.Error("fired under the baseline minimum")
	}
	traffic.base = 100
	traffic.cur = 60
	if next, _ := eval(fired); next.Firing {
		t.Error("did not resolve after traffic recovered")
	}
	traffic.cur = 70
	custom := r
	custom.Threshold, custom.RestartCountThreshold = 80, 50
	if next, _ := eval(custom); !next.Firing {
		t.Error("rule percent 80 should fire at 70 of 100")
	}
	traffic.missing = true
	if next, _ := eval(fired); next.Firing {
		t.Error("missing traffic data must resolve quietly, not fire")
	}
}

func TestEvaluateDomainTrafficDropDomainSelector(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	list := fakeDomainList{domains: []store.ServiceDomain{{Domain: "a.example.com", ServiceName: "web"}, {Domain: "b.example.com", ServiceName: "web"}}}
	e := newTrafficTestEngine(newFakeRuleStore(), nil, nil, &spyNotifier{})
	e.SetTrafficSources(list, nil)
	e.SetDomainTraffic(&fakeTraffic{now: now, cur: 0, base: 100})
	_, notices, err := e.EvaluateDomainTrafficDrop(context.Background(), Rule{ID: "x", Kind: KindDomainTrafficDrop, ResourceID: "domain:B.example.com"}, now)
	if err != nil || len(notices) != 1 {
		t.Fatalf("notices=%v err=%v, want one for b only", notices, err)
	}
}

func TestEvaluateParkedDomainCertExpiry(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	certs := newFakeCertSource()
	certs.seed(t, "old.example.com", now.Add(-80*24*time.Hour), now.Add(5*24*time.Hour))
	certs.seed(t, "fresh.example.com", now.Add(-10*24*time.Hour), now.Add(60*24*time.Hour))
	e := newTrafficTestEngine(newFakeRuleStore(), certs, nil, &spyNotifier{})
	e.SetTrafficSources(fakeDomainList{}, nil)
	e.traffic.parked = fakeParked{domains: []string{"old.example.com", "fresh.example.com", "nocert.example.com"}}
	r := Rule{ID: "pc", Kind: KindParkedDomainCertExpiry}

	fired, notices, err := e.EvaluateParkedDomainCertExpiry(context.Background(), r, now)
	if err != nil || !fired.Firing || len(notices) != 1 {
		t.Fatalf("firing=%v notices=%v err=%v, want only the old domain", fired.Firing, notices, err)
	}
	r.Threshold = 3
	if next, _, _ := e.EvaluateParkedDomainCertExpiry(context.Background(), r, now); next.Firing {
		t.Error("threshold 3 days should not fire at 5 days left")
	}
	// Renewed certificate clears the rule.
	certs.seed(t, "old.example.com", now, now.Add(80*24*time.Hour))
	r.Threshold = 0
	if next, _, _ := e.EvaluateParkedDomainCertExpiry(context.Background(), fired, now); next.Firing {
		t.Error("did not clear after renewal")
	}
	e.traffic.parked = fakeParked{}
	if next, n, err := e.EvaluateParkedDomainCertExpiry(context.Background(), fired, now); err != nil || next.Firing || len(n) != 0 {
		t.Error("no parked domains must be quiet")
	}
}

func TestEngineTick_DomainLifecycleKindsSkipWithoutSources(t *testing.T) {
	rules := newFakeRuleStore(
		Rule{ID: "td", Kind: KindDomainTrafficDrop, Enabled: true},
		Rule{ID: "pc", Kind: KindParkedDomainCertExpiry, Enabled: true},
	)
	e := newTrafficTestEngine(rules, nil, nil, &spyNotifier{})
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
}
