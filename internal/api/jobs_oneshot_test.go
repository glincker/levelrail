package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetOneShotJob_InterruptedByRestartIsFailedNotLost(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	if err := db.SaveOneShotJob(t.Context(), store.OneShotJob{
		ID: "jb_old", App: "web", State: "Running", Completions: 3, Parallelism: 1, Succeeded: 1, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/jobs/runs/jb_old", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got oneShotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "Failed" || got.Succeeded != 1 || !strings.Contains(got.Message, "restarted") || got.FinishedAt == nil {
		t.Fatalf("job = %+v, want Failed with progress kept and a restart reason", got)
	}
}

func TestHandleGetOneShotJob_UnknownIs404(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/jobs/runs/jb_none", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
