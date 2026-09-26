package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestSLOFlagsApply(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		flags   sloFlags
		wantErr string
		wantObj string
	}{
		{"availability", "slo_burn", sloFlags{target: 99.9}, "", "availability"},
		{"latency", "slo_burn", sloFlags{target: 99, latencyMs: 300}, "", "latency"},
		{"missing target", "slo_burn", sloFlags{}, "--slo is required", ""},
		{"flags on another kind", "threshold", sloFlags{target: 99.9}, "slo_burn only", ""},
		{"no flags on another kind", "threshold", sloFlags{}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req createAlertRuleRequest
			err := tc.flags.apply(tc.kind, &req)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantObj == "" {
				if req.SLO != nil {
					t.Fatalf("SLO set on %s rule", tc.kind)
				}
				return
			}
			if req.SLO == nil || req.SLO.Objective != tc.wantObj || req.SLO.Target != tc.flags.target {
				t.Fatalf("SLO = %+v", req.SLO)
			}
		})
	}
}

func TestAlertRuleConditionSLO(t *testing.T) {
	got := alertRuleCondition(alertRuleResource{Kind: "slo_burn", SLO: &apiclient.AlertSLOConfig{Objective: "latency", Target: 99, LatencyMs: 250}})
	if got != "99% of requests under 250ms, 30d budget" {
		t.Fatalf("condition = %q", got)
	}
}

func TestPrintSLOPreview(t *testing.T) {
	var buf bytes.Buffer
	printSLOPreview(&buf, &apiclient.SLOPreviewResource{Config: apiclient.AlertSLOConfig{Objective: "availability", Target: 99.9}})
	if !strings.Contains(buf.String(), "no request traffic") {
		t.Fatalf("no-traffic output = %q", buf.String())
	}

	buf.Reset()
	printSLOPreview(&buf, &apiclient.SLOPreviewResource{
		Config: apiclient.AlertSLOConfig{Objective: "availability", Target: 99.9}, HasTraffic: true, BudgetRemaining: 0.42, BudgetRequests: 5000,
		Tiers: []apiclient.SLOTierStatus{
			{Name: "page_fast", Factor: 14.4, LongSeconds: 3600, ShortSeconds: 300, Page: true, LongBurn: 20, ShortBurn: 18, Firing: true},
			{Name: "ticket_slow", Factor: 1, LongSeconds: 259200, ShortSeconds: 21600, LongBurn: 0.2, ShortBurn: 0.1},
		},
	})
	out := buf.String()
	for _, want := range []string{"42.0%", "FIRING (page)", "14.4x over 1h and 5m", "1x over 3d and 6h", "ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintRecentChanges(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	printRecentChanges(&buf, &apiclient.RecentChangesResource{WindowSeconds: 1800}, "")
	if !strings.Contains(buf.String(), "no changes in the last 30m0s") {
		t.Fatalf("empty output = %q", buf.String())
	}

	buf.Reset()
	printRecentChanges(&buf, &apiclient.RecentChangesResource{
		WindowSeconds: 1800, Total: 4,
		Changes: []apiclient.RecentChange{
			{At: at, Title: "Deploy to app@sha256:abc", Detail: "digest abc", Actor: "bob", LikelyCause: true},
			{At: at, Title: "Env changed: A", Keys: []string{"A"}},
		},
	}, "  ")
	out := buf.String()
	for _, want := range []string{"<- likely cause", "by bob", "[A]", "and 2 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	buf.Reset()
	printRecentChanges(&buf, nil, "")
	if buf.Len() != 0 {
		t.Fatal("nil changes must print nothing")
	}
}
