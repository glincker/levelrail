package alerting

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var logRuleClock = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func logRulePolicy() LogRulePolicy {
	return LogRulePolicy{EvalEvery: time.Minute, MaxWindow: time.Hour, MaxRows: 100, Samples: 3}
}

func seededLogStore(t *testing.T, lines []telemetry.LogEntry) *telemetry.DB {
	t.Helper()
	db, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.WriteLogBatch(context.Background(), lines); err != nil {
		t.Fatal(err)
	}
	return db
}

func logLine(age time.Duration, msg string) telemetry.LogEntry {
	return telemetry.LogEntry{ResourceID: "service:web", Stream: "stderr", Timestamp: logRuleClock.Add(-age), Message: msg}
}

func TestLogRuleConfigValidate(t *testing.T) {
	p := logRulePolicy()
	tests := []struct {
		name string
		kind Kind
		cfg  LogRuleConfig
		ok   bool
	}{
		{"pattern ok", KindLogPattern, LogRuleConfig{Query: "timeout", Count: 5}, true},
		{"pattern needs matcher", KindLogPattern, LogRuleConfig{Count: 5}, false},
		{"pattern needs count", KindLogPattern, LogRuleConfig{Query: "x"}, false},
		{"count over row cap", KindLogPattern, LogRuleConfig{Query: "x", Count: 101}, false},
		{"window over cap", KindLogPattern, LogRuleConfig{Query: "x", Count: 1, WindowSeconds: 7200}, false},
		{"bad regex", KindLogPattern, LogRuleConfig{Regex: "(", Count: 1}, false},
		{"bad field", KindLogPattern, LogRuleConfig{Fields: []string{"nope"}, Count: 1}, false},
		{"bad level", KindLogPattern, LogRuleConfig{Level: "loud", Count: 1}, false},
		{"rate ok", KindLogLevelRate, LogRuleConfig{PerMinute: 3}, true},
		{"rate needs threshold", KindLogLevelRate, LogRuleConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate(tt.kind, p)
			if (err == nil) != tt.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestEvaluateLogRule_PatternFiresWithSamples(t *testing.T) {
	var lines []telemetry.LogEntry
	for i := 0; i < 6; i++ {
		lines = append(lines, logLine(time.Duration(i+1)*30*time.Second, "ERROR payment timeout password=hunter2 order "+string(rune('0'+i))))
	}
	lines = append(lines, logLine(10*time.Second, "INFO all good"), logLine(40*time.Minute, "ERROR payment timeout too old"))
	db := seededLogStore(t, lines)

	rule := Rule{ID: "r1", Kind: KindLogPattern, ResourceID: "service:web", Enabled: true,
		LogRule: &LogRuleConfig{Query: "payment timeout", Count: 5, WindowSeconds: 300}}
	next, res, err := EvaluateLogRule(context.Background(), db, rule, logRulePolicy(), logRuleClock)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Firing || res.Matched != 6 {
		t.Fatalf("firing=%v matched=%d, want firing with 6 matches (old line is outside the window)", next.Firing, res.Matched)
	}
	if len(res.Samples) != 3 {
		t.Fatalf("samples = %d, want 3", len(res.Samples))
	}
	for _, s := range res.Samples {
		if strings.Contains(s, "hunter2") {
			t.Fatalf("sample leaks secret: %q", s)
		}
	}
	if next.LastValue == nil || *next.LastValue != 6 {
		t.Fatalf("LastValue = %v", next.LastValue)
	}

	rule.LogRule.Count = 7
	next, _, _ = EvaluateLogRule(context.Background(), db, rule, logRulePolicy(), logRuleClock)
	if next.Firing {
		t.Fatal("must not fire below the count")
	}
}

func TestMatchLogRule_RegexAndFieldFilters(t *testing.T) {
	entries := []telemetry.LogEntry{
		{Timestamp: logRuleClock, Message: "GET /a 500", Structured: true, FieldsJSON: `{"status":500,"path":"/a"}`},
		{Timestamp: logRuleClock, Message: "GET /b 200", Structured: true, FieldsJSON: `{"status":200,"path":"/b"}`},
		{Timestamp: logRuleClock, Message: "GET /c 503", Structured: true, FieldsJSON: `{"status":503,"path":"/c"}`},
	}
	cfg := LogRuleConfig{Fields: []string{"status>=500"}, Regex: `/[ab] `, Count: 1}
	res, err := MatchLogRule(KindLogPattern, cfg, entries, false, logRuleClock, logRulePolicy())
	if err != nil || res.Matched != 1 || !res.Satisfied {
		t.Fatalf("res=%+v err=%v, want exactly the /a line", res, err)
	}
}

func TestMatchLogRule_LevelRate(t *testing.T) {
	var entries []telemetry.LogEntry
	for i := 0; i < 20; i++ {
		entries = append(entries, telemetry.LogEntry{Timestamp: logRuleClock.Add(-time.Duration(i) * 10 * time.Second), Message: "ERROR boom"})
	}
	entries = append(entries, telemetry.LogEntry{Timestamp: logRuleClock, Message: "INFO ok"})
	cfg := LogRuleConfig{PerMinute: 3, WindowSeconds: 300}
	res, _ := MatchLogRule(KindLogLevelRate, cfg, entries, false, logRuleClock, logRulePolicy())
	if want := 4.0; res.Value != want || !res.Satisfied {
		t.Fatalf("value=%v satisfied=%v, want rate %v firing (20 errors over 5m)", res.Value, res.Satisfied, want)
	}
	cfg.PerMinute = 5
	if res, _ = MatchLogRule(KindLogLevelRate, cfg, entries, false, logRuleClock, logRulePolicy()); res.Satisfied {
		t.Fatal("rate 4/min must not fire at threshold 5")
	}
}

func TestMatchLogRule_TruncatedRateUsesCoveredSpan(t *testing.T) {
	var entries []telemetry.LogEntry
	for i := 0; i < 60; i++ {
		entries = append(entries, telemetry.LogEntry{Timestamp: logRuleClock.Add(-time.Duration(60-i) * time.Second), Message: "ERROR x"})
	}
	res, _ := MatchLogRule(KindLogLevelRate, LogRuleConfig{PerMinute: 10, WindowSeconds: 3600}, entries, true, logRuleClock, logRulePolicy())
	if res.Value < 50 || !strings.Contains(res.Notice, "only the newest") {
		t.Fatalf("value=%v notice=%q: a truncated read must rate over the span it covered", res.Value, res.Notice)
	}
}

func TestEngine_LogRule_CadenceCooldownAndSamples(t *testing.T) {
	logs := &fakeLogsSource{}
	for i := 0; i < 3; i++ {
		logs.entries = append(logs.entries, telemetry.LogEntry{Timestamp: logRuleClock.Add(-time.Duration(i) * time.Second), Message: "ERROR db down"})
	}
	rule := Rule{ID: "lp", Kind: KindLogPattern, ResourceID: "service:web", Enabled: true,
		LogRule: &LogRuleConfig{Regex: "db down", Count: 2, WindowSeconds: 300, CooldownSeconds: 600}}
	rules := newFakeRuleStore(rule)
	spy := &spyNotifier{}
	e := newTestEngine(rules, nil, logs, nil, spy)
	e.logRule.policy = logRulePolicy()

	next, skip, err := e.evaluateLogRule(context.Background(), rule, logRuleClock)
	if err != nil || skip || !next.Firing {
		t.Fatalf("first eval: firing=%v skip=%v err=%v", next.Firing, skip, err)
	}
	if notice, samples := e.logRule.take("lp"); !strings.Contains(notice, "3 matching lines") || len(samples) != 3 {
		t.Fatalf("notice=%q samples=%v", notice, samples)
	}

	if _, skip, _ = e.evaluateLogRule(context.Background(), rule, logRuleClock.Add(10*time.Second)); !skip {
		t.Fatal("a second evaluation inside the cadence must be skipped")
	}

	resolved := rule
	resolved.Firing = false
	again, skip, _ := e.evaluateLogRule(context.Background(), resolved, logRuleClock.Add(2*time.Minute))
	if skip || again.Firing {
		t.Fatalf("re-fire inside the cooldown must be suppressed: skip=%v firing=%v", skip, again.Firing)
	}
	later, _, _ := e.evaluateLogRule(context.Background(), resolved, logRuleClock.Add(11*time.Minute))
	if !later.Firing {
		t.Fatal("after the cooldown the rule fires again")
	}
}

func TestEngine_Tick_LogPatternDispatchAttachesSamples(t *testing.T) {
	logs := &fakeLogsSource{entries: []telemetry.LogEntry{
		{Timestamp: time.Now(), Message: "ERROR cannot connect to redis"},
		{Timestamp: time.Now(), Message: "ERROR cannot connect to redis"},
	}}
	rule := Rule{ID: "lp", Kind: KindLogPattern, ResourceID: "service:web", Enabled: true,
		LogRule: &LogRuleConfig{Query: "redis", Count: 2}}
	rules := newFakeRuleStore(rule)
	spy := &spyNotifier{}
	e := newTestEngine(rules, nil, logs, nil, spy)

	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("notify calls = %d, want 1", len(calls))
	}
	if len(calls[0].LogLines) != 2 || !strings.Contains(calls[0].LogNotice, "2 matching lines") {
		t.Fatalf("event = %+v", calls[0])
	}
	if body := summaryBody(calls[0]); !strings.Contains(body, "Matching sample 2 log lines") {
		t.Fatalf("body missing samples:\n%s", body)
	}
}
