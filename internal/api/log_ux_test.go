package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestTrimLogBudgetNowDrainsOverBudgetApp(t *testing.T) {
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "trim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	big := strings.Repeat("x", 100<<10)
	now := time.Now()
	var batch []telemetry.LogEntry
	for i := 0; i < 15; i++ {
		batch = append(batch, telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(time.Duration(i) * time.Second), Message: big})
	}
	if err := tel.WriteLogBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithLogBudgetStore(tel))
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPost, "/api/v1/apps/web/log-budget/trim", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"trimmed_lines":0`) {
		t.Fatalf("no budget should trim nothing: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPut, "/api/v1/apps/web/log-budget", `{"max_mb":1}`); rec.Code != http.StatusOK {
		t.Fatalf("set budget = %d", rec.Code)
	}
	rec := do(http.MethodPost, "/api/v1/apps/web/log-budget/trim", "")
	var res LogTrimResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("trim = %d %s", rec.Code, rec.Body)
	}
	if res.TrimmedLines == 0 || res.StillOver {
		t.Fatalf("expected a drained trim, got %+v", res)
	}
}

func TestQueryLogsOrderRelevance(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	now := time.Now().UTC().Truncate(time.Second)
	if err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-3 * time.Minute), Message: "timeout timeout timeout upstream"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-2 * time.Minute), Message: "a long line about many other things that mentions timeout once among lots of words"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "timeout"},
	}); err != nil {
		t.Fatal(err)
	}
	get := func(order string) []string {
		rec := httptest.NewRecorder()
		url := "/api/v1/apps/web/logs?q=timeout&limit=2&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339) + order
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		var out struct {
			Entries []struct {
				Message string `json:"message"`
			} `json:"entries"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Total != 3 {
			t.Fatalf("total = %d", out.Total)
		}
		var msgs []string
		for _, e := range out.Entries {
			msgs = append(msgs, e.Message)
		}
		return msgs
	}
	newest := get("")
	if len(newest) != 2 || newest[1] != "timeout" {
		t.Fatalf("newest order = %v", newest)
	}
	ranked := get("&order=relevance")
	if len(ranked) != 2 || ranked[0] != "timeout timeout timeout upstream" || ranked[1] != "timeout" {
		t.Fatalf("relevance order = %v", ranked)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs?order=bogus", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad order = %d", rec.Code)
	}
}
