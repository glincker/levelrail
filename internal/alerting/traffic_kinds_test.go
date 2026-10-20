package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDomainList struct{ domains []store.ServiceDomain }

func (f fakeDomainList) ListServiceDomains(context.Context) ([]store.ServiceDomain, error) {
	return f.domains, nil
}

type fakeACME struct{ failing map[string]string }

func (f fakeACME) ACMEFailing(domain string) (string, bool) {
	e, ok := f.failing[domain]
	return e, ok
}

func newTrafficTestEngine(rules *fakeRuleStore, certs CertSource, checker DomainCheckSource, spy *spyNotifier) *Engine {
	return NewEngine(rules, nil, nil, nil, certs, nil, 0, time.Hour, nil, 0, 0, nil, 0, 0, nil, checker, time.Millisecond, nil, 0, func(Rule) Notifier { return spy }, nil)
}

func TestEvaluateCertExpiring(t *testing.T) {
	now := time.Now()
	certs := newFakeCertSource()
	certs.seed(t, "soon.example.com", now.Add(-60*24*time.Hour), now.Add(5*24*time.Hour))
	certs.seed(t, "later.example.com", now.Add(-10*24*time.Hour), now.Add(40*24*time.Hour))
	cases := []struct {
		name    string
		rule    Rule
		firing  bool
		notices int
	}{
		{"default 14 days", Rule{ID: "r", Kind: KindCertExpiring}, true, 1},
		{"60 days catches both", Rule{ID: "r", Kind: KindCertExpiring, Threshold: 60}, true, 2},
		{"3 days catches none", Rule{ID: "r", Kind: KindCertExpiring, Threshold: 3}, false, 0},
		{"selector other domain", Rule{ID: "r", Kind: KindCertExpiring, ResourceID: "domain:later.example.com"}, false, 0},
		{"selector matching domain", Rule{ID: "r", Kind: KindCertExpiring, ResourceID: "domain:soon.example.com"}, true, 1},
		{"app scoped id watches all", Rule{ID: "r", Kind: KindCertExpiring, ResourceID: "service:web"}, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, notices, err := EvaluateCertExpiring(context.Background(), certs, tc.rule, now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if next.Firing != tc.firing || len(notices) != tc.notices {
				t.Errorf("firing=%v notices=%v, want %v and %d", next.Firing, notices, tc.firing, tc.notices)
			}
		})
	}
}

func TestEvaluateCertRenewalStalled(t *testing.T) {
	now := time.Now()
	certs := newFakeCertSource()
	certs.seed(t, "ok.example.com", now.Add(-10*24*time.Hour), now.Add(70*24*time.Hour))
	r := Rule{ID: "stall", Kind: KindCertRenewalStalled}
	e := newTrafficTestEngine(newFakeRuleStore(r), certs, nil, &spyNotifier{})

	next, notices, err := e.EvaluateCertRenewalStalled(context.Background(), r, now)
	if err != nil || next.Firing || len(notices) != 0 {
		t.Fatalf("healthy: firing=%v notices=%v err=%v", next.Firing, notices, err)
	}

	certs.seed(t, "dead.example.com", now.Add(-100*24*time.Hour), now.Add(-time.Hour))
	next, notices, _ = e.EvaluateCertRenewalStalled(context.Background(), r, now)
	if !next.Firing || len(notices) != 1 || !strings.Contains(notices[0], "dead.example.com") {
		t.Errorf("expired: firing=%v notices=%v", next.Firing, notices)
	}
}

func TestCertRenewalStalled_PersistentACMEFailure(t *testing.T) {
	now := time.Now()
	r := Rule{ID: "stall", Kind: KindCertRenewalStalled}
	e := newTrafficTestEngine(newFakeRuleStore(r), newFakeCertSource(), nil, &spyNotifier{})
	e.SetTrafficSources(fakeDomainList{domains: []store.ServiceDomain{{Domain: "x.example.com", ServiceName: "web"}}},
		fakeACME{failing: map[string]string{"x.example.com": "connection refused"}})

	if next, _, _ := e.EvaluateCertRenewalStalled(context.Background(), r, now); next.Firing {
		t.Fatal("fired on the first failure, want it to wait for the stalled threshold")
	}
	next, notices, _ := e.EvaluateCertRenewalStalled(context.Background(), r, now.Add(2*time.Hour))
	if !next.Firing || len(notices) != 1 || !strings.Contains(notices[0], "connection refused") {
		t.Errorf("after threshold: firing=%v notices=%v", next.Firing, notices)
	}
}

func TestEvaluateDomainNotResolving(t *testing.T) {
	now := time.Now()
	checker := &fakeDomainCheckSource{status: map[string]string{"dark.example.com": domainHealthStatusNotResolving}}
	r := Rule{ID: "nr", Kind: KindDomainNotResolving, ForDuration: 30 * time.Minute}
	e := newTrafficTestEngine(newFakeRuleStore(r), nil, checker, &spyNotifier{})
	e.SetTrafficSources(fakeDomainList{domains: []store.ServiceDomain{
		{Domain: "dark.example.com", ServiceName: "web"}, {Domain: "fine.example.com", ServiceName: "web"},
	}}, nil)

	steps := []struct {
		after  time.Duration
		firing bool
	}{{0, false}, {10 * time.Minute, false}, {31 * time.Minute, true}}
	for _, s := range steps {
		next, notices, err := e.EvaluateDomainNotResolving(context.Background(), r, now.Add(s.after))
		if err != nil {
			t.Fatal(err)
		}
		if next.Firing != s.firing {
			t.Errorf("after %s: firing=%v notices=%v, want %v", s.after, next.Firing, notices, s.firing)
		}
	}

	checker.status["dark.example.com"] = "connected"
	if next, _, _ := e.EvaluateDomainNotResolving(context.Background(), r, now.Add(40*time.Minute)); next.Firing {
		t.Error("still firing after the domain resolved")
	}
}

func TestEngineTick_TrafficKinds(t *testing.T) {
	now := time.Now()
	certs := newFakeCertSource()
	certs.seed(t, "soon.example.com", now.Add(-60*24*time.Hour), now.Add(2*24*time.Hour))
	rules := newFakeRuleStore(
		Rule{ID: "exp", Kind: KindCertExpiring, Enabled: true},
		Rule{ID: "dns", Kind: KindDomainNotResolving, Enabled: true},
	)
	spy := &spyNotifier{}
	e := newTrafficTestEngine(rules, certs, &fakeDomainCheckSource{}, spy)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !rules.get("exp").Firing {
		t.Error("cert_expiring did not fire")
	}
	calls := spy.calls()
	if len(calls) != 1 || len(calls[0].CertNotices) != 1 {
		t.Fatalf("events = %+v, want one cert_expiring event with a notice", calls)
	}
	if rules.get("dns").Firing {
		t.Error("domain_not_resolving fired without a domain source; it should be skipped")
	}
}
