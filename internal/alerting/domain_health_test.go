package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAppDomainSource is an in-memory AppDomainSource, the same
// hand-written-fake pattern every other package in this codebase uses
// instead of a mocking framework.
type fakeAppDomainSource struct {
	services map[string]*store.DesiredService
}

func newFakeAppDomainSource(services ...store.DesiredService) *fakeAppDomainSource {
	m := make(map[string]*store.DesiredService, len(services))
	for i := range services {
		svc := services[i]
		m[svc.Name] = &svc
	}
	return &fakeAppDomainSource{services: m}
}

func (f *fakeAppDomainSource) GetDesiredService(_ context.Context, name string) (*store.DesiredService, error) {
	svc, ok := f.services[name]
	if !ok {
		return nil, store.ErrServiceNotFound
	}
	return svc, nil
}

// fakeDomainCheckSource is an in-memory DomainCheckSource: status is
// keyed by domain, a domain missing from status defaults to "connected"
// so a test only has to specify the domains it cares about. calls
// records every domain actually checked, in order, so a throttle test can
// assert how many real checks happened.
type fakeDomainCheckSource struct {
	status map[string]string
	calls  []string
}

func (f *fakeDomainCheckSource) CheckDomainStatus(_ context.Context, domain string) (string, error) {
	f.calls = append(f.calls, domain)
	if status, ok := f.status[domain]; ok {
		return status, nil
	}
	return "connected", nil
}

func TestEvaluateDomainHealth_AllConnected_NotFiring(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com", "b.example.com"}})
	checker := &fakeDomainCheckSource{}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: every domain resolves correctly")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
	if got.LastValue == nil || *got.LastValue != 0 {
		t.Errorf("LastValue = %v, want 0", got.LastValue)
	}
}

func TestEvaluateDomainHealth_OneNotResolving_Fires(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com", "b.example.com"}})
	checker := &fakeDomainCheckSource{status: map[string]string{"b.example.com": domainHealthStatusNotResolving}}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: one domain isn't resolving")
	}
	if len(notices) != 1 {
		t.Fatalf("notices = %v, want exactly 1", notices)
	}
	if got.LastValue == nil || *got.LastValue != 1 {
		t.Errorf("LastValue = %v, want 1", got.LastValue)
	}
}

func TestEvaluateDomainHealth_ResolvesElsewhere_Fires(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com"}})
	checker := &fakeDomainCheckSource{status: map[string]string{"a.example.com": domainHealthStatusResolvesElsewhere}}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: the domain resolves somewhere other than this control plane")
	}
	if len(notices) != 1 {
		t.Fatalf("notices = %v, want exactly 1", notices)
	}
}

func TestEvaluateDomainHealth_Unconfigured_NotFiring(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com"}})
	checker := &fakeDomainCheckSource{status: map[string]string{"a.example.com": "unconfigured"}}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: \"unconfigured\" (no APP_PUBLIC_HOST) is inconclusive, not unhealthy")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
}

func TestEvaluateDomainHealth_AppDeleted_GoesQuietNoError(t *testing.T) {
	apps := newFakeAppDomainSource() // empty: the watched app no longer exists
	checker := &fakeDomainCheckSource{}
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:gone", Enabled: true, Firing: true, FiringSince: &firingSince}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v, want nil for a deleted app", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false once the watched app is gone")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
}

func TestEvaluateDomainHealth_NoDomainsConfigured_NotFiring(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web"}) // no domains
	checker := &fakeDomainCheckSource{}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}

	got, notices, err := EvaluateDomainHealth(context.Background(), apps, checker, r, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateDomainHealth() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: app has no domains configured")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
	if len(checker.calls) != 0 {
		t.Errorf("checker was called %d times, want 0 domains checked", len(checker.calls))
	}
}

func TestEngine_Tick_DomainHealthFires_NotifiesOnce(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com"}})
	checker := &fakeDomainCheckSource{status: map[string]string{"a.example.com": domainHealthStatusNotResolving}}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithDomainHealth(rules, apps, checker, time.Millisecond, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("Notify called %d times, want 1", len(calls))
	}
	if len(calls[0].DomainHealthNotices) != 1 {
		t.Errorf("DomainHealthNotices = %v, want exactly 1 on a firing event", calls[0].DomainHealthNotices)
	}
	if !rules.get("r1").Firing {
		t.Error("persisted state Firing = false, want true")
	}
}

func TestEngine_Tick_DomainHealthResolved_NoNotice(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com"}})
	checker := &fakeDomainCheckSource{} // resolves fine now
	firingSince := time.Now().Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithDomainHealth(rules, apps, checker, time.Millisecond, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 || !calls[0].Resolved {
		t.Fatalf("calls = %+v, want one Resolved=true event", calls)
	}
	if len(calls[0].DomainHealthNotices) != 0 {
		t.Errorf("DomainHealthNotices = %v, want none on a resolved event", calls[0].DomainHealthNotices)
	}
}

func TestEngine_Tick_DomainHealth_NoSourceConfigured_Skipped(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, nil, nil, spy) // no domain apps/checker wired

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times with no domain check source configured, want 0", len(calls))
	}
}

func TestEngine_Tick_DomainHealth_ThrottledSecondTick_NoRealCheck(t *testing.T) {
	apps := newFakeAppDomainSource(store.DesiredService{Name: "web", Domains: []string{"a.example.com"}})
	checker := &fakeDomainCheckSource{}
	r := Rule{ID: "r1", Kind: KindDomainHealth, ResourceID: "service:web", Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	// An interval far longer than this test takes: the second Tick must
	// not perform a second real check.
	engine := newTestEngineWithDomainHealth(rules, apps, checker, time.Hour, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	if len(checker.calls) != 1 {
		t.Errorf("checker was called %d times across two ticks inside the check interval, want 1", len(checker.calls))
	}
}
