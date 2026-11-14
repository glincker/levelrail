package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func soloApprove(t *testing.T, rt *Router, cookie *http.Cookie, body string) int {
	t.Helper()
	return iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/approve", body), nil)
}

func setSoloSetting(t *testing.T, rt *Router, on bool) {
	t.Helper()
	raw := json.RawMessage("false")
	if on {
		raw = json.RawMessage("true")
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyStacksSoloSelfApprove, raw, "test"); err != nil {
		t.Fatal(err)
	}
}

func soloFixture(t *testing.T) (*Router, *http.Cookie) {
	t.Helper()
	rt, _, cookie := stackFixture(t)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", `{"require_approval":true}`), nil); code != http.StatusOK {
		t.Fatalf("enable approval: %d", code)
	}
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	return rt, cookie
}

func TestStackSoloSelfApproveOffRefuses(t *testing.T) {
	rt, cookie := soloFixture(t)
	if code := soloApprove(t, rt, cookie, `{"confirm_stack":"shop-prod"}`); code != http.StatusForbidden {
		t.Fatalf("setting off = %d", code)
	}
}

func TestStackSoloSelfApproveTwoRootsRefuses(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	seedApproverToken(t, db, "approver", []string{AbilityRoot})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", `{"require_approval":true}`), nil); code != http.StatusOK {
		t.Fatalf("enable approval: %d", code)
	}
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	setSoloSetting(t, rt, true)
	if code := soloApprove(t, rt, cookie, `{"confirm_stack":"shop-prod"}`); code != http.StatusForbidden {
		t.Fatalf("two root users = %d", code)
	}
}

func TestStackSoloSelfApprove(t *testing.T) {
	rt, cookie := soloFixture(t)
	setSoloSetting(t, rt, true)
	if code := soloApprove(t, rt, cookie, ""); code != http.StatusForbidden {
		t.Fatalf("no typed name = %d", code)
	}
	if code := soloApprove(t, rt, cookie, `{"confirm_stack":"other"}`); code != http.StatusForbidden {
		t.Fatalf("wrong typed name = %d", code)
	}
	if code := soloApprove(t, rt, cookie, `{"confirm_stack":"shop-prod"}`); code != http.StatusOK {
		t.Fatalf("typed name = %d", code)
	}
	n := 0
	for _, e := range listAuditLog(t, rt, cookie) {
		if e.Action == auditStackSoloSelfApprove {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("solo self approve audit rows = %d", n)
	}
}
