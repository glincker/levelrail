package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCITemplatesFillInTheApp(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/ci/templates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var res ciTemplatesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, tmpl := range res.Templates {
		ids[tmpl.ID] = tmpl.Auth
		if !strings.Contains(tmpl.Content, "web") || strings.Contains(tmpl.Content, "my-app") {
			t.Errorf("%s: app name not filled in", tmpl.ID)
		}
	}
	for id, auth := range map[string]string{"github": "oidc", "gitlab": "oidc", "circleci": "oidc", "jenkins": "token", "buildkite": "token"} {
		if ids[id] != auth {
			t.Errorf("provider %s auth = %q, want %q", id, ids[id], auth)
		}
	}
	if strings.Join(res.DeployerAbilities, ",") != "read,deploy" || !res.NeedsDomain {
		t.Errorf("resource = abilities %v needsDomain %v", res.DeployerAbilities, res.NeedsDomain)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/ci/templates", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d, want 404", rec.Code)
	}
}
