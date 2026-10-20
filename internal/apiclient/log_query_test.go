package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseLogTime(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"30m", now.Add(-30 * time.Minute), false},
		{"2026-09-26T10:00:00Z", time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC), false},
		{"yesterday", time.Time{}, true},
	}
	for _, tc := range tests {
		got, err := ParseLogTime(tc.in, now)
		if (err != nil) != tc.wantErr || (!tc.wantErr && !got.Equal(tc.want)) {
			t.Errorf("ParseLogTime(%q) = %v, %v", tc.in, got, err)
		}
	}
}

func TestLogMaxBytesFromEnv(t *testing.T) {
	tests := []struct {
		val  string
		set  bool
		want int
	}{
		{"", false, DefaultLogMaxBytes},
		{"2048", true, 2048},
		{"0", true, DefaultLogMaxBytes},
		{"junk", true, DefaultLogMaxBytes},
	}
	for _, tc := range tests {
		got := LogMaxBytesFromEnv(func(string) (string, bool) { return tc.val, tc.set })
		if got != tc.want {
			t.Errorf("env %q: got %d want %d", tc.val, got, tc.want)
		}
	}
}

func TestDeployWindow(t *testing.T) {
	t0 := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	now := t0.Add(5 * time.Hour)
	attempts := []DeployAttemptResource{
		{ID: "c", StartedAt: t0.Add(3 * time.Hour)},
		{ID: "a", StartedAt: t0},
		{ID: "b", StartedAt: t0.Add(time.Hour)},
	}
	tests := []struct {
		id       string
		from, to time.Time
		wantErr  bool
	}{
		{"a", t0, t0.Add(time.Hour), false},
		{"b", t0.Add(time.Hour), t0.Add(3 * time.Hour), false},
		{"c", t0.Add(3 * time.Hour), now, false},
		{"zzz", time.Time{}, time.Time{}, true},
	}
	for _, tc := range tests {
		from, to, err := DeployWindow(attempts, tc.id, now)
		if (err != nil) != tc.wantErr || !from.Equal(tc.from) || !to.Equal(tc.to) {
			t.Errorf("DeployWindow(%s) = %v %v %v", tc.id, from, to, err)
		}
	}
}

func TestBuildLogExcerptCapsBytesAndReportsCounts(t *testing.T) {
	ts := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	entries := make([]LogEntryResource, 200)
	for i := range entries {
		entries[i] = LogEntryResource{Timestamp: ts.Add(time.Duration(i) * time.Second), Message: strings.Repeat("x", 100), Level: "error"}
	}
	ex := BuildLogExcerpt("web", ts, ts.Add(time.Hour), entries, 1812, 2048)
	raw, _ := json.Marshal(ex)
	if len(raw) > 2048 {
		t.Errorf("excerpt is %d bytes, cap 2048", len(raw))
	}
	if ex.Shown == 0 || ex.Shown >= 200 || ex.Matched != 1812 {
		t.Errorf("shown=%d matched=%d", ex.Shown, ex.Matched)
	}
	if !strings.Contains(ex.Notice, "of 1,812 matching lines") || !strings.Contains(ex.Notice, "since/until") {
		t.Errorf("notice = %q", ex.Notice)
	}
	if !strings.HasSuffix(ex.Lines[len(ex.Lines)-1], "x...") && !strings.Contains(ex.Lines[len(ex.Lines)-1], "ERROR") {
		t.Errorf("newest line missing: %q", ex.Lines[len(ex.Lines)-1])
	}
	if !strings.Contains(ex.Lines[len(ex.Lines)-1], "12:03:19") {
		t.Errorf("last line should be the newest entry: %q", ex.Lines[len(ex.Lines)-1])
	}
}

func TestBuildLogExcerptNoTruncation(t *testing.T) {
	ts := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	ex := BuildLogExcerpt("web", ts, ts, []LogEntryResource{{Timestamp: ts, Message: "hi"}}, 1, DefaultLogMaxBytes)
	if ex.Notice != "" || ex.Shown != 1 || ex.Lines[0] != "12:00:00 hi" {
		t.Errorf("excerpt = %+v", ex)
	}
}

func TestQueryLogsCompactSendsFilters(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	var gotQuery map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/apps/web/deploy-attempts":
			_ = json.NewEncoder(w).Encode([]DeployAttemptResource{{ID: "d1", StartedAt: now.Add(-2 * time.Hour)}, {ID: "d2", StartedAt: now.Add(-time.Hour)}})
		case "/api/v1/apps/web/logs":
			gotQuery = map[string]string{}
			for k := range r.URL.Query() {
				gotQuery[k] = r.URL.Query().Get(k)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total": 7, "entries": []map[string]any{{"timestamp": now.Format(time.RFC3339), "message": "boom", "level": "error"}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "t")

	ex, err := c.QueryLogsCompact(context.Background(), LogQuery{App: "web", Deploy: "d1", Level: "error", Text: "boom", MaxLines: 5}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery["level"] != "error" || gotQuery["q"] != "boom" || gotQuery["limit"] != "5" {
		t.Errorf("query = %v", gotQuery)
	}
	if gotQuery["from"] != now.Add(-2*time.Hour).Format(time.RFC3339) || gotQuery["to"] != now.Add(-time.Hour).Format(time.RFC3339) {
		t.Errorf("deploy window not applied: %v", gotQuery)
	}
	if ex.Matched != 7 || ex.Shown != 1 || !strings.Contains(ex.Notice, "1 of 7") {
		t.Errorf("excerpt = %+v", ex)
	}

	for _, q := range []LogQuery{{App: ""}, {App: "web", Since: "bogus"}, {App: "web", Deploy: "nope"}, {App: "web", Since: "1h", Until: "2h"}} {
		if _, err := c.QueryLogsCompact(context.Background(), q, 0, now); err == nil {
			t.Errorf("expected an error for %+v", q)
		}
	}
}
