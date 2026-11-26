package dbadvisor

import (
	"strings"
	"testing"
)

func testThresholds() Thresholds {
	return Thresholds{UnusedIndexMinBytes: 1000, SeqScanMinRows: 100, SeqScanRatioPercent: 50, DeadTuplePercent: 20,
		CacheHitMinPercent: 95, IdleInTxMinutes: 5, SlowStatementMs: 100, MaxFindings: 50}
}

func TestBuildUnusedIndexes(t *testing.T) {
	th := testThresholds()
	rows := []UnusedIndexRow{
		{Schema: "public", Table: "t", Index: "big_unused", IdxScan: 0, Bytes: 5000},
		{Schema: "public", Table: "t", Index: "small", IdxScan: 0, Bytes: 10},
		{Schema: "public", Table: "t", Index: "used", IdxScan: 3, Bytes: 9000},
	}
	got := BuildUnusedIndexes(rows, th)
	if len(got) != 1 || got[0].Object != "public.big_unused" {
		t.Fatalf("findings = %+v", got)
	}
	if !strings.Contains(got[0].Suggestion, `DROP INDEX CONCURRENTLY "public"."big_unused"`) || !strings.Contains(got[0].Suggestion, "branch") {
		t.Fatalf("suggestion = %q", got[0].Suggestion)
	}
}

func TestBuildMissingFKIndexes(t *testing.T) {
	th := testThresholds()
	got := BuildMissingFKIndexes([]FKRow{
		{Schema: "public", Table: "orders", Constraint: "fk_u", Columns: []string{"user id", "x"}, RefTable: "users", LiveTup: 500},
		{Schema: "public", Table: "tiny", Constraint: "fk_t", Columns: []string{"a"}, LiveTup: 3},
		{Schema: "public", Table: "none", Constraint: "fk_n"},
	}, th)
	if len(got) != 2 {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Severity != SeverityWarn || got[1].Severity != SeverityInfo {
		t.Fatalf("severities = %s %s", got[0].Severity, got[1].Severity)
	}
	if !strings.Contains(got[0].Suggestion, `CREATE INDEX CONCURRENTLY ON "public"."orders" ("user id", "x")`) {
		t.Fatalf("suggestion = %q", got[0].Suggestion)
	}
}

func TestBuildSeqScans(t *testing.T) {
	th := testThresholds()
	got := BuildSeqScans([]TableStatRow{
		{Schema: "public", Table: "hot", SeqScan: 90, IdxScan: 10, LiveTup: 1000, SeqTupRead: 90000},
		{Schema: "public", Table: "cheap", SeqScan: 1, IdxScan: 0, LiveTup: 1000, SeqTupRead: 0},
		{Schema: "public", Table: "indexed", SeqScan: 10, IdxScan: 90, LiveTup: 1000},
		{Schema: "public", Table: "small", SeqScan: 100, IdxScan: 0, LiveTup: 5},
		{Schema: "public", Table: "never", SeqScan: 0, IdxScan: 0, LiveTup: 1000},
	}, th)
	if len(got) != 1 || got[0].Object != "public.hot" || got[0].Evidence["seq_scan_percent"] != 90.0 {
		t.Fatalf("findings = %+v", got)
	}
}

func TestBuildBloat(t *testing.T) {
	th := testThresholds()
	got := BuildBloat([]TableStatRow{
		{Schema: "public", Table: "warn", LiveTup: 1000, DeadTup: 250},
		{Schema: "public", Table: "crit", LiveTup: 1000, DeadTup: 900},
		{Schema: "public", Table: "clean", LiveTup: 1000, DeadTup: 10},
		{Schema: "public", Table: "small", LiveTup: 5, DeadTup: 4},
	}, th)
	if len(got) != 2 || got[0].Severity != SeverityWarn || got[1].Severity != SeverityCritical {
		t.Fatalf("findings = %+v", got)
	}
}

func TestBuildCacheHit(t *testing.T) {
	th := testThresholds()
	if got := BuildCacheHit(CacheRow{BlksHit: 99, BlksRead: 1}, th); len(got) != 0 {
		t.Fatalf("healthy ratio flagged: %+v", got)
	}
	if got := BuildCacheHit(CacheRow{}, th); len(got) != 0 {
		t.Fatalf("no traffic flagged: %+v", got)
	}
	got := BuildCacheHit(CacheRow{BlksHit: 90, BlksRead: 10}, th)
	if len(got) != 1 || got[0].Severity != SeverityWarn {
		t.Fatalf("findings = %+v", got)
	}
	if got := BuildCacheHit(CacheRow{BlksHit: 10, BlksRead: 90}, th); got[0].Severity != SeverityCritical {
		t.Fatalf("severity = %s", got[0].Severity)
	}
}

func TestBuildStatements(t *testing.T) {
	th := testThresholds()
	off := BuildStatements(StatementsState{Installed: true}, nil, th)
	if len(off) != 1 || off[0].Kind != KindStatementsOff || !strings.Contains(off[0].Suggestion, "restart") {
		t.Fatalf("not enabled finding = %+v", off)
	}
	on := StatementsState{Installed: true, Preloaded: true}
	rows := []StatementRow{
		{QueryID: "1", Calls: 5, MeanMs: 450, TotalMs: 2250, Query: "SELECT * FROM t WHERE token = 'abc123'\n  AND a = $1"},
		{QueryID: "2", Calls: 5, MeanMs: 120, TotalMs: 600, Query: "SELECT 1"},
		{QueryID: "3", Calls: 5, MeanMs: 5, TotalMs: 25, Query: "SELECT 2"},
	}
	got := BuildStatements(on, rows, th)
	if len(got) != 2 || got[0].Severity != SeverityWarn || got[1].Severity != SeverityInfo {
		t.Fatalf("findings = %+v", got)
	}
	text, _ := got[0].Evidence["statement"].(string)
	if strings.Contains(text, "abc123") || strings.Contains(text, "\n") {
		t.Fatalf("statement not cleaned: %q", text)
	}
	if got := BuildStatements(on, nil, th); len(got) != 0 {
		t.Fatalf("empty rows gave %+v", got)
	}
}

func TestBuildSessions(t *testing.T) {
	th := testThresholds()
	got := BuildSessions([]SessionRow{
		{PID: 1, State: "idle in transaction", XactAgeSec: 400, Query: "UPDATE t SET p = 'secret'"},
		{PID: 2, State: "active", XactAgeSec: 700},
		{PID: 3, State: "active", XactAgeSec: 30},
	}, th)
	if len(got) != 2 || got[0].Severity != SeverityWarn || got[1].Severity != SeverityCritical {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Title != "Session is idle in transaction" {
		t.Fatalf("title = %q", got[0].Title)
	}
	if s, _ := got[0].Evidence["statement"].(string); strings.Contains(s, "secret") {
		t.Fatalf("literal leaked: %q", s)
	}
}

func TestCleanTextTruncates(t *testing.T) {
	if got := CleanText(strings.Repeat("a", 1000)); len([]rune(got)) != textMax {
		t.Fatalf("len = %d", len([]rune(got)))
	}
}

func TestOrderCapsAndSorts(t *testing.T) {
	fs := []Finding{{Severity: SeverityInfo, Object: "a"}, {Severity: SeverityCritical, Object: "b"}, {Severity: SeverityWarn, Object: "c"}}
	got, trunc := Order(fs, 2)
	if !trunc || len(got) != 2 || got[0].Severity != SeverityCritical || got[1].Severity != SeverityWarn {
		t.Fatalf("got %+v trunc %v", got, trunc)
	}
}

func TestSafeReason(t *testing.T) {
	if SafeReason(errTest("boom")) != "query failed" {
		t.Fatal("generic error not mapped")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// The advisor's own statements run under a cursor and must never be reported back as slow.
func TestStatementsSQLSkipsOwnCursor(t *testing.T) {
	if !strings.Contains(statementsSQL(testThresholds()), "NOT LIKE 'DECLARE lr_cursor%'") {
		t.Fatal("own statements are not excluded")
	}
}

func TestCleanTextMasksLiteralForms(t *testing.T) {
	cases := map[string]string{
		"plain":      "SELECT 1 WHERE a = 'topsecret'",
		"escaped":    `SELECT 1 WHERE a = 'it''s topsecret'`,
		"backslash":  `SELECT E'a\'topsecret' FROM t`,
		"cut":        "SELECT 1 WHERE k = '-----BEGIN KEY topsecret",
		"dollar":     "SELECT $$topsecret 'x'$$, $tag$topsecret$tag$",
		"dollar cut": "SELECT $body$topsecret and more",
		"digits":     "SELECT 1 WHERE card = 4111111111111111 AND topsecret",
		"role ddl":   "ALTER ROLE r WITH LOGIN PASSWORD topsecret",
	}
	for name, in := range cases {
		out := CleanText(in)
		if name == "digits" {
			if strings.Contains(out, "4111") {
				t.Errorf("%s: %q", name, out)
			}
			continue
		}
		if strings.Contains(out, "topsecret") || strings.Contains(out, "BEGIN KEY") {
			t.Errorf("%s leaked: %q", name, out)
		}
	}
	if got := CleanText("SELECT * FROM t WHERE id = $1 LIMIT 10"); got != "SELECT * FROM t WHERE id = $1 LIMIT 10" {
		t.Errorf("parameters and short numbers must survive: %q", got)
	}
}
