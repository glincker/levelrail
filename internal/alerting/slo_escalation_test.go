package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func addMinutes(f *sloFake, from, to int, total, bad float64) {
	now := time.Now()
	for i := from; i < to; i++ {
		at := now.Add(-time.Duration(i) * time.Minute).Add(-time.Second)
		f.add("web", telemetry.MetricHTTPRequests, at, total)
		if bad > 0 {
			f.add("web", telemetry.MetricHTTPResponses5xx, at, bad)
		}
	}
}

func TestSLOEscalationFromTicketToPageNotifiesAgain(t *testing.T) {
	ctx := context.Background()
	rule := Rule{ID: "slo1", Name: "web slo", Kind: KindSLOBurn, ResourceID: "service:web", Enabled: true,
		SLO: &SLOConfig{Objective: SLOAvailability, Target: 99.9}}
	f := &sloFake{}
	addMinutes(f, 0, 72*60, 1000, 1.5) // 1.5x for three days: ticket only
	spy := &spyNotifier{}
	e := newTestEngine(newFakeRuleStore(rule), f, nil, nil, spy)
	e.slo.EvalEvery = 0

	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	calls := spy.calls()
	if len(calls) != 1 || calls[0].Rule.Severity != SeverityWarning {
		t.Fatalf("first tick calls = %+v", calls)
	}

	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spy.calls()) != 1 {
		t.Fatal("an unchanged ticket-level burn must not notify again")
	}

	addMinutes(f, 0, 60, 0, 50) // a fresh outage: 5% failing on top
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	calls = spy.calls()
	if len(calls) != 2 || calls[1].Rule.Severity != SeverityCritical || !strings.Contains(calls[1].SLONotice, "page") {
		t.Fatalf("escalation calls = %+v", calls)
	}

	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spy.calls()) != 2 {
		t.Fatal("a page-level burn must notify once, not on every tick")
	}
}

func TestSLOFirstSightingWhileFiringDoesNotRepage(t *testing.T) {
	rule := Rule{ID: "slo1", Name: "web slo", Kind: KindSLOBurn, ResourceID: "service:web", Enabled: true, Firing: true,
		SLO: &SLOConfig{Objective: SLOAvailability, Target: 99.9}}
	f := &sloFake{}
	addMinutes(f, 0, 60, 1000, 50)
	spy := &spyNotifier{}
	e := newTestEngine(newFakeRuleStore(rule), f, nil, nil, spy)
	e.slo.EvalEvery = 0
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(spy.calls()) != 0 {
		t.Fatalf("a rule already firing before this process started must not be re-paged: %+v", spy.calls())
	}
}
