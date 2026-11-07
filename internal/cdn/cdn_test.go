package cdn

import (
	"context"
	"errors"
	"testing"
)

func TestHistoryKeepsNewestPerApp(t *testing.T) {
	h := NewHistory(2)
	for _, tr := range []string{TriggerPublish, TriggerDeploy, TriggerRollback} {
		h.Add("web", PurgeRecord{Trigger: tr, OK: true})
	}
	got := h.List("web", 0)
	if len(got) != 2 || got[0].Trigger != TriggerRollback || got[1].Trigger != TriggerDeploy {
		t.Fatalf("history = %+v", got)
	}
	if len(h.List("other", 0)) != 0 {
		t.Fatal("other app should be empty")
	}
}

func TestFakeProviderSatisfiesContract(t *testing.T) {
	var p Provider = &Fake{ID: "bunny", Mode: "strict", PurgeErr: errors.New("boom")}
	if p.Name() != "bunny" {
		t.Fatalf("name = %q", p.Name())
	}
	if err := p.PurgeHosts(context.Background(), "example.com", []string{"a.example.com"}); err == nil {
		t.Fatal("want purge error")
	}
	if m, _ := p.SSLMode(context.Background(), "example.com"); m != "strict" {
		t.Fatalf("mode = %q", m)
	}
}

func TestSummarizeWorstWins(t *testing.T) {
	r := SetupResult{Steps: []Step{{Status: StatusOK}, {Status: StatusWarn}}}
	r.Summarize()
	if r.Status != StatusWarn {
		t.Fatalf("status = %q", r.Status)
	}
	r.Steps = append(r.Steps, Step{Status: StatusFail})
	r.Summarize()
	if r.Status != StatusFail {
		t.Fatalf("status = %q", r.Status)
	}
}
