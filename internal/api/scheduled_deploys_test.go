package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScheduledDeployRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/apps/web/scheduled-deploys"},
		{http.MethodGet, "/api/v1/apps/web/scheduled-deploys"},
		{http.MethodGet, "/api/v1/apps/web/scheduled-deploys/sdep_1"},
		{http.MethodDelete, "/api/v1/apps/web/scheduled-deploys/sdep_1"},
	})
}

func TestScheduledDeployCRUD(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	bad := []struct{ name, body string }{
		{"neither", `{"run_at":"` + at + `"}`},
		{"both", `{"run_at":"` + at + `","image":"a:1","ref":"main"}`},
		{"bad image", `{"run_at":"` + at + `","image":"Not Valid"}`},
		{"past", `{"run_at":"2020-01-01T00:00:00Z","image":"a:1"}`},
		{"not a time", `{"run_at":"soon","image":"a:1"}`},
		{"ref without git source", `{"run_at":"` + at + `","ref":"main"}`},
	}
	for _, tc := range bad {
		if rec := do(http.MethodPost, "/api/v1/apps/web/scheduled-deploys", tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}
	if rec := do(http.MethodPost, "/api/v1/apps/missing/scheduled-deploys", `{"run_at":"`+at+`","image":"a:1"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing app status = %d, want 404", rec.Code)
	}

	rec := do(http.MethodPost, "/api/v1/apps/web/scheduled-deploys", `{"run_at":"`+at+`","image":"img:v2"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	var created scheduledDeployResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Status != "pending" || created.Kind != "image" {
		t.Fatalf("created = %+v, %v", created, err)
	}

	seedGitSourceForSchedule(t, db)
	if rec := do(http.MethodPost, "/api/v1/apps/web/scheduled-deploys", `{"run_at":"`+at+`","ref":"main"}`); rec.Code != http.StatusCreated {
		t.Fatalf("git create status = %d: %s", rec.Code, rec.Body.String())
	}

	var list []scheduledDeployResource
	rec = do(http.MethodGet, "/api/v1/apps/web/scheduled-deploys?status=pending", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v: %s", len(list), err, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/apps/web/scheduled-deploys?limit=0", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("limit=0 status = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/v1/apps/web/scheduled-deploys/"+created.ID, ""); rec.Code != http.StatusOK {
		t.Errorf("get status = %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/v1/apps/web/scheduled-deploys/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get unknown status = %d, want 404", rec.Code)
	}

	rec = do(http.MethodDelete, "/api/v1/apps/web/scheduled-deploys/"+created.ID, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"canceled"`) {
		t.Fatalf("cancel = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, "/api/v1/apps/web/scheduled-deploys/"+created.ID, ""); rec.Code != http.StatusConflict {
		t.Errorf("second cancel status = %d, want 409", rec.Code)
	}
}
