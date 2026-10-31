package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

func TestResilienceReport(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	var empty chaosReport
	rec := serve(http.MethodGet, "/api/v1/resilience", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &empty)
	if len(empty.Latest) != 0 || len(empty.Unmeasured) != len(chaos.Catalog()) {
		t.Fatalf("empty report = %+v", empty)
	}

	body, _ := json.Marshal(chaosRecordRequest{Results: []chaos.Result{
		{Scenario: chaos.KillContainer, Status: chaos.StatusPassed, Summary: "ok", RecoveryMS: 3525, RanAt: time.Now()},
	}})
	if rec = serve(http.MethodPost, "/api/v1/resilience/results", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
	}
	var rep chaosReport
	_ = json.Unmarshal(serve(http.MethodGet, "/api/v1/resilience", "").Body.Bytes(), &rep)
	if len(rep.Latest) != 1 || rep.Latest[0].RecoveryMS != 3525 || len(rep.Unmeasured) != len(chaos.Catalog())-1 {
		t.Fatalf("report = %+v", rep)
	}

	for _, bad := range []string{
		`{"results":[]}`,
		`{"results":[{"scenario":"nope","status":"passed"}]}`,
		`{"results":[{"scenario":"kill-container","status":"great"}]}`,
	} {
		if rec = serve(http.MethodPost, "/api/v1/resilience/results", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want 400", bad, rec.Code)
		}
	}
}
