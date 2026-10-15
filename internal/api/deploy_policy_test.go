package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/policy"
)

func TestDeployPolicyEnforcedAndOverridden(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	put := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/deploy-policies/org/org",
		`{"rules":{"allowed_registries":{"mode":"enforce","values":["ghcr.io"]},"require_readiness_probe":{"mode":"warn"}}}`))
	if put.Code != http.StatusOK {
		t.Fatalf("put = %d %s", put.Code, put.Body)
	}

	blocked := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2"}`))
	if blocked.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blocked = %d %s", blocked.Code, blocked.Body)
	}
	var body policyViolationBody
	_ = json.Unmarshal(blocked.Body.Bytes(), &body)
	if body.Code != policy.CodeViolation || len(body.Violations) != 1 || body.Violations[0].Code != policy.CodeRegistryNotAllowed || body.Violations[0].NextAction == "" {
		t.Fatalf("body = %+v", body)
	}

	noReason := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2","override_policy":true}`))
	if noReason.Code != http.StatusBadRequest || !strings.Contains(noReason.Body.String(), policyOverrideReasonCode) {
		t.Fatalf("override without reason = %d %s", noReason.Code, noReason.Body)
	}
	ok := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2","override_policy":true,"override_policy_reason":"hotfix"}`))
	if ok.Code >= http.StatusBadRequest {
		t.Fatalf("override = %d %s", ok.Code, ok.Body)
	}
}

func TestDeployPolicyDryRunAndValidation(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	check := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/deploy-policies/check",
		`{"scope":"org","scope_id":"org","rules":{"max_replicas":{"mode":"enforce","limit":1},"require_readiness_probe":{"mode":"enforce"}}}`))
	if check.Code != http.StatusOK {
		t.Fatalf("check = %d %s", check.Code, check.Body)
	}
	var res policyCheckResult
	_ = json.Unmarshal(check.Body.Bytes(), &res)
	if res.Checked != 1 || res.Failing != 1 || len(res.Apps) != 1 || res.Apps[0].App != "web" {
		t.Fatalf("result = %+v", res)
	}

	bad := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/deploy-policies/org/org", `{"rules":{"max_cpu":{"mode":"enforce"}}}`))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid rules = %d, want 400", bad.Code)
	}
	list := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/deploy-policies", ""))
	if list.Code != http.StatusOK || strings.TrimSpace(list.Body.String()) != "[]" {
		t.Fatalf("list = %d %s", list.Code, list.Body)
	}
}
