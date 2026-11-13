package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestStackErrorCodes(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK {
		t.Fatalf("seed apply: %d", code)
	}
	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"unknown stack", http.MethodGet, "/api/v1/stacks/nope", "", http.StatusNotFound, "not_found"},
		{"bad revision number", http.MethodGet, "/api/v1/stacks/shop-prod/revisions/zero", "", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad against", http.MethodGet, "/api/v1/stacks/shop-prod/revisions/1/diff?against=x", "", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad body", http.MethodPost, "/api/v1/stacks", "{", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad stack name", http.MethodPost, "/api/v1/stacks", `{"name":"Bad Name!","project":"shop"}`, http.StatusUnprocessableEntity, iac.CodeInvalidRequest},
		{"duplicate stack", http.MethodPost, "/api/v1/stacks", `{"name":"shop-prod","project":"shop"}`, http.StatusConflict, iac.CodeOwnershipConflict},
	}
	for _, c := range cases {
		var out iacErrorBody
		got := iacCall(t, rt, authedRequest(t, cookie, c.method, c.path, c.body), &out)
		if got != c.status || out.Code != c.code || out.Error == "" {
			t.Errorf("%s: status %d body %+v, want %d %s", c.name, got, out, c.status, c.code)
		}
	}
}

func TestPlanHashVersionCodes(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	var plan iacPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil)), &plan); code != http.StatusOK || plan.Plan == nil {
		t.Fatalf("plan: %d", code)
	}
	on := true
	iac.SetV1HashAcceptedSource(func() bool { return on })
	t.Cleanup(func() { iac.SetV1HashAcceptedSource(nil) })
	apply := func(hash string, out *iacErrorBody) int {
		body := stackBody(t, stackTestDocs("ghcr.io/acme/web:1.2.3"), map[string]any{"expected_plan_hash": hash})
		return iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", body), out)
	}
	var out iacErrorBody
	if code := apply("deadbeefdeadbeef", &out); code != http.StatusConflict || out.Code != iac.CodePlanStale {
		t.Fatalf("v1 mismatch on = %d %+v", code, out)
	}
	if code := apply("v2:wrong", &out); code != http.StatusConflict || out.Code != iac.CodePlanStale {
		t.Fatalf("v2 mismatch = %d %+v", code, out)
	}
	on = false
	out = iacErrorBody{}
	if code := apply("deadbeefdeadbeef", &out); code != http.StatusConflict || out.Code != iac.CodePlanHashVersion {
		t.Fatalf("v1 off = %d %+v", code, out)
	}
	on = true
	if code := apply(iac.PlanHashV1(*plan.Plan), &iacErrorBody{}); code != http.StatusOK {
		t.Fatalf("v1 hash accepted = %d", code)
	}
}
