package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every route under /environments/{id} must hide an ungranted environment
// exactly like an unknown id, so a new route cannot skip the grant gate.
func TestEnvironmentGrantGate_EveryIDRouteHidesUngrantedEnvironment(t *testing.T) {
	f := newVisibilityFixture(t)
	cookie := sessionCookieForTest(t, f.rt, f.guest.ID)
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, `{}`))
		return rec
	}
	checked := 0
	for _, route := range openAPIRoutes {
		if !strings.HasPrefix(route.Path, "/api/v1/environments/{id}") {
			continue
		}
		path := func(id string) string {
			return strings.NewReplacer("{id}", id, "{key}", "SOME_KEY").Replace(route.Path)
		}
		hidden := do(route.Method, path("env_production"))
		unknown := do(route.Method, path("env_does_not_exist"))
		if hidden.Code != http.StatusNotFound {
			t.Errorf("%s %s: ungranted environment status = %d, want 404", route.Method, route.Path, hidden.Code)
		}
		if gateErrorText(hidden) != gateErrorText(unknown) {
			t.Errorf("%s %s: ungranted answer %q differs from unknown id answer %q", route.Method, route.Path, hidden.Body.String(), unknown.Body.String())
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("walked %d environment id routes, want at least 10", checked)
	}
}

func TestEnvironmentGrantGate_ListShowsOnlyGrantedEnvironments(t *testing.T) {
	f := newVisibilityFixture(t)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, sessionCookieForTest(t, f.rt, f.guest.ID), http.MethodGet, "/api/v1/environments", ""))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"env_dev"`) || strings.Contains(body, `"env_production"`) {
		t.Fatalf("guest list: status = %d, body = %.400s", rec.Code, body)
	}
}

func TestEnvironmentGrantGate_ProjectListAndCompareHideUngranted(t *testing.T) {
	f := newVisibilityFixture(t)
	env, err := f.db.GetEnvironment(context.Background(), "env_production")
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessionCookieForTest(t, f.rt, f.guest.ID)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		return rec
	}
	base := "/api/v1/projects/" + env.ProjectID + "/environments"
	list := get(base)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"env_dev"`) || strings.Contains(list.Body.String(), `"env_production"`) {
		t.Fatalf("project list: status = %d, body = %.400s", list.Code, list.Body.String())
	}
	hidden := get(base + "/compare?a=env_dev&b=env_production")
	unknown := get(base + "/compare?a=env_dev&b=env_does_not_exist")
	if hidden.Code != http.StatusNotFound || gateErrorText(hidden) != gateErrorText(unknown) {
		t.Fatalf("compare: hidden = %d %q, unknown = %d %q", hidden.Code, hidden.Body.String(), unknown.Code, unknown.Body.String())
	}
}

// gateErrorText drops the per-request id so two refusals compare by meaning.
func gateErrorText(rec *httptest.ResponseRecorder) string {
	var out struct{ Error, Code string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Error + "|" + out.Code
}
