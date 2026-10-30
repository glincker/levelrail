package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const appTypeFixture = `{"name":"batch","workload":"worker","enforcement":"enforce",
 "parameters":{"type":"object","properties":{"n":{"type":"integer","maximum":3}}},
 "binds":{"n":"services.*.replicas"},"build":{"allowed":["dockerfile"]}}`

func TestAppTypesOffReturns404(t *testing.T) {
	t.Setenv("LEVELRAIL_APPTYPE_ENABLED", "")
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	if rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/app-types", "")); rec.Code != http.StatusNotFound {
		t.Fatalf("list with feature off = %d, want 404", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types", appTypeFixture)); rec.Code != http.StatusNotFound {
		t.Fatalf("save with feature off = %d, want 404", rec.Code)
	}
}

func TestAppTypePublishIsImmutableAndCheckWorks(t *testing.T) {
	t.Setenv("LEVELRAIL_APPTYPE_ENABLED", "on")
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types", appTypeFixture)); rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types", `{"name":"x","workload":"web","parameters":{"type":"object","oneOf":[]}}`)); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad schema = %d, want 400", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types/batch/publish", `{}`)); rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d %s", rec.Code, rec.Body)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types/batch/publish", `{"version":1}`)); rec.Code != http.StatusConflict {
		t.Fatalf("republish v1 = %d, want 409", rec.Code)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/app-types/batch/check", `{"spec":{"build":"static","params":{"n":9}}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("check = %d %s", rec.Code, rec.Body)
	}
	var res appTypeCheckResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Version != 1 || !res.WouldFail || len(res.Violations) != 2 || !strings.HasPrefix(res.Violations[0].Code, "apptype_") {
		t.Fatalf("result = %+v", res)
	}
}
