package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeMailer struct {
	configured bool
	sent       int
}

func (m *fakeMailer) Configured(context.Context) bool { return m.configured }
func (m *fakeMailer) Send(context.Context, string, string, string) error {
	m.sent++
	return nil
}

func TestRenderWeeklyDigest(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	f := WeeklyFacts{
		Since: now.AddDate(0, 0, -7), Until: now, Apps: 4, Deploys: 9, DeploysFailed: 2,
		Restarts:      []NamedCount{{Name: "web", Count: 5}},
		CertsExpiring: []NamedCount{{Name: "a.example.com", Count: 6}},
		BackupsOK:     7, BackupsFailed: 1, DiskWorstPercent: 83, DiskWorstNode: "n1",
	}
	subject, text, html := RenderWeeklyDigest(f)
	for _, want := range []string{"Apps: 4", "Deploys: 9 (2 failed)", "web 5", "a.example.com in 6 days", "7 succeeded, 1 failed", "83% on n1"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(subject, "2026-11-23") || !strings.Contains(html, "<li>") {
		t.Errorf("subject %q html %q", subject, html)
	}
}

func TestWeeklyDigestNoMailWithoutEmail(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 5, 0, 0, time.UTC) // a Monday
	m := &fakeMailer{}
	sentDay := ""
	w := &WeeklyDigest{
		Mailer:    m,
		Schedule:  func(context.Context) (int, int, bool) { return 1, 8, true },
		Recipient: func(context.Context) string { return "admin@example.com" },
		Last:      func(context.Context) string { return sentDay },
		SetLast:   func(_ context.Context, d string) { sentDay = d },
	}
	w.Tick(context.Background(), now)
	if m.sent != 0 || sentDay != "" {
		t.Fatalf("sent %d with email unconfigured", m.sent)
	}
	m.configured = true
	w.Tick(context.Background(), now)
	w.Tick(context.Background(), now)
	if m.sent != 1 {
		t.Fatalf("sent %d, want exactly 1 per day", m.sent)
	}
	w.Schedule = func(context.Context) (int, int, bool) { return 1, 8, false }
	sentDay = ""
	w.Tick(context.Background(), now)
	if m.sent != 1 {
		t.Fatal("digest sent while off")
	}
}

func TestDefaultSpecsIncludeCrashloopAndErrorRate(t *testing.T) {
	var crash, rate bool
	for _, s := range DefaultRuleSpecs(DefaultsConfigFromEnv(func(string) string { return "" })) {
		switch s.Key {
		case DefaultCrashloopRuleID:
			crash = s.Rule.Kind == KindCrashloop && s.Rule.RestartCountThreshold == 3
		case DefaultErrorRateRuleID:
			rate = s.Rule.Kind == KindThreshold && s.Rule.Threshold == 5
		}
	}
	if !crash || !rate {
		t.Fatalf("crashloop=%v errorRate=%v", crash, rate)
	}
}

func TestEvaluateCrashloopAll(t *testing.T) {
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	tr := NewRestartTracker()
	tr.Observe("api", "c1", now.Add(-time.Minute))
	for i := 0; i < 4; i++ {
		tr.Observe("api", "c1", now.Add(-time.Duration(i+1)*time.Minute))
	}
	r := Rule{ID: DefaultCrashloopRuleID, ResourceID: DefaultRuleResource, Kind: KindCrashloop, RestartCountThreshold: 3, RestartWindow: 10 * time.Minute}
	next, notice := EvaluateCrashloopAll(tr, r, 3, now)
	if !next.Firing || !strings.Contains(notice, "api") {
		t.Fatalf("firing=%v notice=%q", next.Firing, notice)
	}
	if next, _ := EvaluateCrashloopAll(tr, r, 50, now); next.Firing {
		t.Fatal("live threshold 50 should not fire")
	}
}
