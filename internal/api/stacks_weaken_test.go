package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStackApplyWeakenNeedsHumanRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	svc := runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt.runtimeSettings = svc
	cookie := loginTestSession(t, rt, db)
	if code, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", `{"name":"base","scope":"install"}`)); code != http.StatusCreated {
		t.Fatalf("create install stack = %d %s", code, out)
	}
	doc := "version: 1\nkind: Setting\nmetadata:\n  name: " + runtimesettings.KeyWebhookReplayProtection + "\nspec:\n  value: false\n"
	plain := stackBody(t, doc, nil)
	weaken := stackBody(t, doc, map[string]any{"allow_weaken": true})
	approver := seedApproverToken(t, db, "approver", []string{AbilityRoot})

	if _, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/base/plan", plain)); !strings.Contains(out, "weakens a security protection; apply with allow_weaken") {
		t.Fatalf("plan without allow_weaken must be an error item: %s", out)
	}
	if _, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/base/plan", weaken)); !strings.Contains(out, iac.SettingWeakenReason) {
		t.Fatalf("plan with allow_weaken must mark the weakening: %s", out)
	}
	for _, path := range []string{"/api/v1/stacks/base/plan", "/api/v1/stacks/base/apply"} {
		if code, out := rawCall(t, rt, bearerRequest(http.MethodPost, path, weaken, approver)); code != http.StatusForbidden {
			t.Fatalf("root token %s with allow_weaken = %d %s", path, code, out)
		}
	}

	var pending stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/base/apply", weaken), &pending); code != http.StatusAccepted || !pending.PendingApproval {
		t.Fatalf("session apply = %d %+v", code, pending)
	}
	s, err := db.GetStackByName(t.Context(), "base")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := db.GetStackRevision(t.Context(), s.ID, 1)
	if err != nil || !strings.Contains(string(rev.ResultJSON), `"allow_weaken":true`) {
		t.Fatalf("approval must store allow_weaken: %s %v", rev.ResultJSON, err)
	}

	if code, out := rawCall(t, rt, bearerRequest(http.MethodPost, "/api/v1/stacks/base/revisions/1/approve", "", approver)); code != http.StatusForbidden {
		t.Fatalf("token approve of a weakening = %d %s", code, out)
	}
	if got, _ := svc.Get(t.Context(), runtimesettings.KeyWebhookReplayProtection); strings.TrimSpace(string(got.Value)) != "true" {
		t.Fatalf("refused approve changed the setting: %s", got.Value)
	}

	second := storeUserWithAbilitiesForTest(t, db, "second-root@example.test", []string{AbilityRoot})
	var done stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, sessionCookieForTest(t, rt, second.ID), http.MethodPost, "/api/v1/stacks/base/revisions/1/approve", ""), &done); code != http.StatusOK {
		t.Fatalf("human root approve = %d %+v", code, done)
	}
	if got, _ := svc.Get(t.Context(), runtimesettings.KeyWebhookReplayProtection); strings.TrimSpace(string(got.Value)) != "false" {
		t.Fatalf("approved weakening did not apply: %s %+v", got.Value, done.Result)
	}
	entries, _ := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{Action: auditStackSettingWeaken})
	if len(entries) != 1 || !strings.Contains(entries[0].Detail, runtimesettings.KeyWebhookReplayProtection) {
		t.Fatalf("want one stack.setting_weaken row, got %+v", entries)
	}
}
