package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseAdvisor_RefusesOtherEngines(t *testing.T) {
	for _, engine := range []string{store.EngineMySQL, store.EngineMongoDB, store.EngineRedis} {
		rt, db, _ := newViewerTestRouter(t, engine)
		cookie := loginTestSession(t, rt, db)
		rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/advisor", "")
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Postgres only") {
			t.Errorf("%s: status = %d, body = %s", engine, rec.Code, rec.Body.String())
		}
	}
	rt, db, _ := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)
	if rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/missing/advisor", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing database status = %d", rec.Code)
	}
}

// Every check gets "[]": the list checks find nothing and the statements check cannot decode it,
// which must surface as one check_failed finding instead of failing the response.
func TestDatabaseAdvisor_PartialResultAndCache(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	viewerOverviews.invalidate("main")
	cookie := loginTestSession(t, rt, db)
	fake.out = "json\n[]\n"

	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/advisor", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Engine   string `json:"engine"`
		Cached   bool   `json:"cached"`
		Findings []struct {
			Kind     string         `json:"kind"`
			Evidence map[string]any `json:"evidence"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Engine != "postgres" || got.Cached || len(got.Findings) != 1 || got.Findings[0].Kind != "check_failed" {
		t.Fatalf("response = %s", rec.Body.String())
	}
	if reason, _ := got.Findings[0].Evidence["reason"].(string); reason != "query failed" {
		t.Errorf("reason = %q", reason)
	}
	calls := fake.calls
	rec = viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/advisor", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cached":true`) || fake.calls != calls {
		t.Errorf("second call was not served from the cache: calls %d -> %d, body = %s", calls, fake.calls, rec.Body.String())
	}
}
