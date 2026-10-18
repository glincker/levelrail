package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dockerlint"
)

func TestDockerLintEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/docker/lint", body))
		return rec
	}

	if rec := do(`{"dockerfile":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty dockerfile = %d, want 400", rec.Code)
	}
	rec := do(`{"dockerfile":"FROM node\nCMD [\"x\"]\n","has_dockerignore":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lint = %d %s", rec.Code, rec.Body.String())
	}
	var res dockerlint.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 || !strings.Contains(res.FixedDockerfile, "USER "+dockerlint.NonRootUser) {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestDockerHealthWithoutClient(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/docker/health", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d", rec.Code)
	}
	var res dockerHealthResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Reachable {
		t.Fatalf("no client configured must not report reachable: %+v", res)
	}
}

func TestImageStatusUnknownApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nope/image-status", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("image-status = %d, want 404", rec.Code)
	}
}
