package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestClassifyLogLevel(t *testing.T) {
	tests := []struct {
		name string
		e    telemetry.LogEntry
		want string
	}{
		{"json string level", telemetry.LogEntry{Structured: true, FieldsJSON: `{"level":"Warning","msg":"x"}`}, levelWarn},
		{"json severity", telemetry.LogEntry{Structured: true, FieldsJSON: `{"severity":"ERROR"}`}, levelError},
		{"pino numeric", telemetry.LogEntry{Structured: true, FieldsJSON: `{"level":50}`}, levelError},
		{"json without level", telemetry.LogEntry{Structured: true, FieldsJSON: `{"msg":"x"}`}, ""},
		{"upper token", telemetry.LogEntry{Message: "2026-01-01T00:00:00Z ERROR db down"}, levelError},
		{"logfmt", telemetry.LogEntry{Message: `ts=1 level=warn msg="slow"`}, levelWarn},
		{"bracketed", telemetry.LogEntry{Message: "[info] listening"}, levelInfo},
		{"prefix colon", telemetry.LogEntry{Message: "Error: connect ECONNREFUSED"}, levelError},
		{"panic is fatal", telemetry.LogEntry{Message: "PANIC: nil pointer"}, levelFatal},
		{"word in body only", telemetry.LogEntry{Message: "user information was updated"}, ""},
		{"lower error in prose", telemetry.LogEntry{Message: "the error rate is fine"}, ""},
		{"plain", telemetry.LogEntry{Message: "server listening on 3000"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyLogLevel(tc.e); got != tc.want {
				t.Errorf("classifyLogLevel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseLogQueryParams(t *testing.T) {
	if _, err := parseLevelParam("loud"); err == nil {
		t.Error("expected an error for an unknown level")
	}
	if l, err := parseLevelParam("WARNING"); err != nil || l != levelWarn {
		t.Errorf("parseLevelParam(WARNING) = %q, %v", l, err)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		if _, err := parseLimitParam(bad); err == nil {
			t.Errorf("parseLimitParam(%q) should fail", bad)
		}
	}
	if n, err := parseLimitParam(""); err != nil || n != 0 {
		t.Errorf("parseLimitParam(\"\") = %d, %v", n, err)
	}
}

func TestHandleQueryLogs_LevelAndLimit(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	entries := []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-4 * time.Second), Message: "INFO started"},
		{ResourceID: "service:web", Stream: "stderr", Timestamp: now.Add(-3 * time.Second), Message: "ERROR first failure"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-2 * time.Second), Message: "WARN slow"},
		{ResourceID: "service:web", Stream: "stderr", Timestamp: now.Add(-1 * time.Second), Message: "ERROR second failure"},
	}
	if err := tdb.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatalf("seed: %v", err)
	}
	window := "from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)

	tests := []struct {
		name      string
		params    string
		wantCode  int
		wantTotal int
		wantLast  string
		wantLen   int
	}{
		{"no filter", "", 200, 4, "ERROR second failure", 4},
		{"level error", "&level=error", 200, 2, "ERROR second failure", 2},
		{"level warn", "&level=warn", 200, 3, "ERROR second failure", 3},
		{"limit newest", "&limit=1", 200, 4, "ERROR second failure", 1},
		{"level plus limit", "&level=error&limit=1", 200, 2, "ERROR second failure", 1},
		{"bad level", "&level=loud", 400, 0, "", 0},
		{"bad limit", "&limit=0", 400, 0, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs?"+window+tc.params, ""))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if tc.wantCode != http.StatusOK {
				return
			}
			var got logsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Total != tc.wantTotal || len(got.Entries) != tc.wantLen || got.Entries[len(got.Entries)-1].Message != tc.wantLast {
				t.Errorf("total=%d len=%d last=%q", got.Total, len(got.Entries), got.Entries[len(got.Entries)-1].Message)
			}
			if got.Entries[len(got.Entries)-1].Level != levelError {
				t.Errorf("level field = %q, want error", got.Entries[len(got.Entries)-1].Level)
			}
		})
	}
}
