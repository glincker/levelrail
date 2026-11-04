package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestInstallStackCreateIsHumanRootAndRefusesApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tok := seedMatrixToken(t, db, "ci:runner", []string{AbilityRead, AbilityWrite})

	body := `{"name":"nodes-eu","scope":"install"}`
	if code, out := rawCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks", body, tok)); code != http.StatusForbidden {
		t.Fatalf("token create = %d %s", code, out)
	}
	if code, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", `{"name":"bad","scope":"install","project":"shop"}`)); code != http.StatusUnprocessableEntity {
		t.Fatalf("install with project = %d %s", code, out)
	}
	if code, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", `{"name":"p1"}`)); code != http.StatusUnprocessableEntity {
		t.Fatalf("project stack without project = %d %s", code, out)
	}

	var made stackResource
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", body), &made); code != http.StatusCreated {
		t.Fatalf("root create = %d", code)
	}
	if made.Scope != "install" || made.ProjectID != "" || !made.RequireApproval || made.DriftPolicy != "lock" {
		t.Fatalf("install stack = %+v", made)
	}
	var got stackResource
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/nodes-eu", ""), &got); code != http.StatusOK || got.Scope != "install" {
		t.Fatalf("get = %d %+v", code, got)
	}

	code, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/nodes-eu/plan", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil)))
	if !strings.Contains(out, "may own only Node and Setting") {
		t.Fatalf("plan = %d %s", code, out)
	}
}
