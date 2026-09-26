package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/importplan"
)

type fakeImportFiles map[string]string

func (f fakeImportFiles) ReadFile(_ context.Context, _, _, path string) ([]byte, bool, error) {
	v, ok := f[path]
	return []byte(v), ok, nil
}

func TestHandleImportPlan(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.importFiles = func() importplan.FileSource {
		return fakeImportFiles{"Dockerfile": "FROM x\nEXPOSE 3000\n", ".env.example": "API_TOKEN=\n"}
	}
	cookie := loginTestSession(t, rt, db)

	cases := []struct {
		name, body string
		status     int
	}{
		{"repo", `{"text":"https://github.com/acme/site"}`, http.StatusOK},
		{"docker run", `{"text":"docker run -p 80:80 nginx:1"}`, http.StatusOK},
		{"empty", `{"text":""}`, http.StatusBadRequest},
		{"garbage", `{"text":"hello there"}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
		{"file scheme", `{"text":"file:///etc/passwd","kind":"repo"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/plan", c.body))
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, c.status, rec.Body.String())
			}
		})
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/plan", `{"text":"https://github.com/acme/site"}`))
	var plan importplan.DeploymentPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Services[0].Port != 3000 || len(plan.MissingRequiredEnv) != 1 || plan.MissingRequiredEnv[0] != "API_TOKEN" {
		t.Errorf("plan = %+v", plan)
	}
}

func TestHandleImportPlanRequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/plan", nil)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d", rec.Code)
	}
}
