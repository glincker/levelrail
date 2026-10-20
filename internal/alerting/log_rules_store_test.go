package alerting

import (
	"context"
	"testing"
)

func TestLogRuleConfig_PersistsThroughRuleStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	want := LogRuleConfig{Query: "timeout", Regex: `5\d\d`, Fields: []string{"status>=500"}, Level: "error", Count: 9, WindowSeconds: 600, CooldownSeconds: 120}
	r := Rule{ID: "lr1", Name: "timeouts", Kind: KindLogPattern, ResourceID: "service:web", Enabled: true, LogRule: &want}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetRule(ctx, "lr1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LogRule == nil || got.LogRule.Count != 9 || got.LogRule.Regex != want.Regex || len(got.LogRule.Fields) != 1 || got.LogRule.CooldownSeconds != 120 {
		t.Fatalf("round trip lost config: %+v", got.LogRule)
	}
}
