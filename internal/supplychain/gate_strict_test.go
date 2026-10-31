package supplychain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDecideStrictMissingScan(t *testing.T) {
	tests := []struct {
		name   string
		mode   GateMode
		scan   *ScanSummary
		closed bool
		want   string
	}{
		{"missing scan allowed by default", GateBlockOnCritical, nil, false, ActionAllow},
		{"missing scan denied when strict", GateBlockOnCritical, nil, true, ActionBlock},
		{"missing scan allowed in warn mode", GateWarn, nil, true, ActionAllow},
		{"unknown severity ignored by default", GateBlockOnCritical, &ScanSummary{Counts: Counts{Unknown: 1}}, false, ActionAllow},
		{"unknown severity blocks when strict", GateBlockOnCritical, &ScanSummary{Counts: Counts{Unknown: 1}}, true, ActionBlock},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideStrict(tc.mode, tc.scan, "", tc.closed)
			if got.Action != tc.want {
				t.Fatalf("action = %q (%s), want %q", got.Action, got.Reason, tc.want)
			}
		})
	}
}

func TestDecideStrictMissingScanWithOverride(t *testing.T) {
	got := DecideStrict(GateBlockOnCritical, nil, "incident 42", true)
	if got.Action != ActionOverride || !strings.Contains(got.Reason, "scan unavailable") || !strings.Contains(got.Reason, "incident 42") {
		t.Fatalf("decision = %+v, want an override that names the unavailable scan", got)
	}
	if got := DecideStrict(GateBlockOnCritical, nil, "incident 42", false); got.Action != ActionAllow {
		t.Fatalf("fail-open decision = %+v, want allow", got)
	}
}

func TestOverrideWithUnavailableScanIsConsumedOnce(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.FailClosed = true })
	ctx := context.Background()
	h.settings(t, true, "block_on_critical")
	h.runner.exit = 2
	var blocked *BlockedError
	if err := h.svc.AfterBuild(ctx, "web", "da_0", attest(t)); !errors.As(err, &blocked) {
		t.Fatalf("no override must block, got %v", err)
	}
	if _, err := h.svc.ArmOverride(ctx, "web", "incident 42"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatalf("armed override must allow: %v", err)
	}
	rec, _ := h.svc.Get(ctx, "web", "da_1")
	if rec.GateAction != ActionOverride || !strings.Contains(rec.GateReason, "scan unavailable") {
		t.Errorf("record = %q %q", rec.GateAction, rec.GateReason)
	}
	if err := h.svc.AfterBuild(ctx, "web", "da_2", attest(t)); !errors.As(err, &blocked) {
		t.Fatalf("override must be consumed once, got %v", err)
	}
}
