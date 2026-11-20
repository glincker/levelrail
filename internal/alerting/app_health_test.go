package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEvaluateAppUnhealthyScopeAndRecovery(t *testing.T) {
	tr := NewAppHealthTracker()
	now := time.Now()
	tr.Record("web-abc12345", "web", true, 3, now)
	tr.Record("api-abc12345", "api", true, 4, now)

	rule := Rule{ID: "r1", Kind: KindAppUnhealthy, ResourceID: "service:web"}
	next, notice, err := EvaluateAppUnhealthy(context.Background(), tr, rule, now)
	if err != nil || !next.Firing {
		t.Fatalf("want firing, got %v err %v", next.Firing, err)
	}
	if !strings.Contains(notice, "web-abc12345") || strings.Contains(notice, "api-") {
		t.Fatalf("notice not scoped: %q", notice)
	}

	tr.Record("web-abc12345", "web", false, 0, now)
	next, notice, _ = EvaluateAppUnhealthy(context.Background(), tr, next, now.Add(time.Minute))
	if next.Firing || notice != "" {
		t.Fatalf("want resolved, got firing=%v notice=%q", next.Firing, notice)
	}
}

type fakeAdvisories struct{ hits []AdvisoryHit }

func (f fakeAdvisories) AdvisoryHits(context.Context, string) ([]AdvisoryHit, error) {
	return f.hits, nil
}

func TestEvaluateAdvisoryMatchOffByDefaultGate(t *testing.T) {
	src := fakeAdvisories{hits: []AdvisoryHit{{App: "web", AdvisoryID: "ADV-1", Severity: "high"}}}
	now := time.Now()
	rule := Rule{ID: "r2", Kind: KindAdvisoryMatch}
	off, _, _ := EvaluateAdvisoryMatch(context.Background(), src, false, "high", rule, now)
	if off.Firing {
		t.Fatal("disabled setting must not fire")
	}
	on, notice, _ := EvaluateAdvisoryMatch(context.Background(), src, true, "high", rule, now)
	if !on.Firing || !strings.Contains(notice, "ADV-1") {
		t.Fatalf("want firing with id, got %v %q", on.Firing, notice)
	}
}
