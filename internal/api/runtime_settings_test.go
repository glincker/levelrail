package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const rsBase = "/api/v1/settings/runtime"

func rsDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func rsSetting(t *testing.T, rec *httptest.ResponseRecorder) runtimesettings.Setting {
	t.Helper()
	var s runtimesettings.Setting
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode setting: %v: %s", err, rec.Body.String())
	}
	return s
}

func TestRuntimeSettings_ListAndGet(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := rsDo(t, rt, cookie, http.MethodGet, rsBase, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	var list runtimeSettingsList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Settings) != 6 || list.Restart.PendingRestart {
		t.Fatalf("list body = %+v, %v", list, err)
	}

	rec = rsDo(t, rt, cookie, http.MethodGet, rsBase+"/"+runtimesettings.KeyWebhookReplayWindow, "")
	s := rsSetting(t, rec)
	if rec.Code != http.StatusOK || s.Source != runtimesettings.SourceDefault || string(s.Value) != `"10m0s"` || string(s.Default) != `"10m0s"` {
		t.Fatalf("get = %d %+v", rec.Code, s)
	}
	if rec := rsDo(t, rt, cookie, http.MethodGet, rsBase+"/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown key = %d", rec.Code)
	}
}

func TestRuntimeSettings_PutValidationAndWeakening(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	audit := runtimesettings.KeyAuditSensitiveReads

	tests := []struct {
		name     string
		key      string
		body     string
		wantCode int
		wantErr  string
	}{
		{"invalid duration", runtimesettings.KeyWebhookReplayWindow, `{"value":"soon"}`, http.StatusBadRequest, "invalid_value"},
		{"invalid bool", audit, `{"value":5}`, http.StatusBadRequest, "invalid_value"},
		{"relative root", runtimesettings.KeyBindAllowedRoots, `{"value":["rel"]}`, http.StatusBadRequest, "invalid_value"},
		{"weakening needs confirm", audit, `{"value":false}`, http.StatusBadRequest, "confirmation_required"},
		{"shorter window needs confirm", runtimesettings.KeyWebhookReplayWindow, `{"value":"1m"}`, http.StatusBadRequest, "confirmation_required"},
		{"weakening confirmed", audit, `{"value":false,"confirm_weakening":true}`, http.StatusOK, ""},
		{"strengthening needs no confirm", audit, `{"value":true}`, http.StatusOK, ""},
		{"bad json", audit, `{`, http.StatusBadRequest, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+tc.key, tc.body)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if tc.wantErr != "" {
				var e runtimeSettingError
				_ = json.Unmarshal(rec.Body.Bytes(), &e)
				if e.Code != tc.wantErr || e.Field == "" {
					t.Fatalf("error = %+v", e)
				}
			}
		})
	}
	if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/nope", `{"value":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown key = %d", rec.Code)
	}
}

func TestRuntimeSettings_PutResetRoundTripAndAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	key := runtimesettings.KeyBindBlockCredentialDirs

	rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":false,"confirm_weakening":true}`)
	s := rsSetting(t, rec)
	if rec.Code != http.StatusOK || s.Source != runtimesettings.SourceSaved || string(s.Value) != "false" || s.UpdatedAt == nil {
		t.Fatalf("put = %d %+v", rec.Code, s)
	}
	if rt.runtimeSettings.Bool(t.Context(), key) {
		t.Fatal("accessor still true after save")
	}

	rec = rsDo(t, rt, cookie, http.MethodDelete, rsBase+"/"+key, "")
	s = rsSetting(t, rec)
	if rec.Code != http.StatusOK || s.Source != runtimesettings.SourceDefault || string(s.Value) != "true" {
		t.Fatalf("delete = %d %+v", rec.Code, s)
	}

	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	for _, e := range entries {
		if e.Action == AuditActionSettingsChange {
			changes++
			if !strings.Contains(e.Detail, key) || !strings.Contains(e.Detail, `"old"`) || !strings.Contains(e.Detail, `"new"`) {
				t.Fatalf("audit detail = %s", e.Detail)
			}
		}
	}
	if changes != 2 {
		t.Fatalf("settings_change rows = %d, want 2", changes)
	}
}

func TestRuntimeSettings_NonRootForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	const plaintext = "write-token-runtime-settings" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(t.Context(), store.APIToken{
		ID: "tok_rs_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead, AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	key := rsBase + "/" + runtimesettings.KeyAuditSensitiveReads
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, key, `{"value":true}`},
		{http.MethodDelete, key, ""},
		{http.MethodPost, rsBase + "/apply", ""},
		{http.MethodPost, rsBase + "/onboarding/ack", ""},
	} {
		if got := do(c.method, c.path, c.body); got != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, got)
		}
	}
	if got := do(http.MethodGet, rsBase, ""); got != http.StatusOK {
		t.Errorf("read with a write token = %d, want 200", got)
	}
}

func TestRuntimeSettings_ApplyAndPendingRestart(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if rec := rsDo(t, rt, cookie, http.MethodPost, rsBase+"/apply", ""); rec.Code != http.StatusOK {
		t.Fatalf("apply with nothing pending = %d", rec.Code)
	}
	key := runtimesettings.KeyTrustLoopbackForwarded
	rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+key, `{"value":false}`)
	if s := rsSetting(t, rec); rec.Code != http.StatusOK || !s.PendingRestart {
		t.Fatalf("put restart-kind = %d %+v", rec.Code, s)
	}
	var list runtimeSettingsList
	_ = json.Unmarshal(rsDo(t, rt, cookie, http.MethodGet, rsBase, "").Body.Bytes(), &list)
	if !list.Restart.PendingRestart || list.Restart.Supported {
		t.Fatalf("restart = %+v", list.Restart)
	}
	if rec := rsDo(t, rt, cookie, http.MethodPost, rsBase+"/apply", ""); rec.Code != http.StatusConflict {
		t.Fatalf("apply without supervisor = %d, want 409", rec.Code)
	}

	called := make(chan struct{}, 1)
	rt.SetRestarter(Restarter{Supervised: true, Trigger: func() { called <- struct{}{} }})
	if rec := rsDo(t, rt, cookie, http.MethodPost, rsBase+"/apply", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("apply = %d", rec.Code)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("restart not triggered")
	}
}

func TestRuntimeSettings_OnboardingAndSetupGuide(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	var ob runtimeOnboarding
	_ = json.Unmarshal(rsDo(t, rt, cookie, http.MethodGet, rsBase+"/onboarding", "").Body.Bytes(), &ob)
	if ob.Reviewed || len(ob.Items) != 4 {
		t.Fatalf("onboarding = %+v", ob)
	}
	guideState := func() string {
		rec := rsDo(t, rt, cookie, http.MethodGet, "/api/v1/setup/guide", "")
		var g struct {
			Steps []struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"steps"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
			t.Fatalf("guide: %v: %s", err, rec.Body.String())
		}
		for _, s := range g.Steps {
			if s.ID == "security-review" {
				return s.State
			}
		}
		t.Fatal("security-review step missing")
		return ""
	}
	if got := guideState(); got != "todo" {
		t.Fatalf("step before ack = %s", got)
	}
	rec := rsDo(t, rt, cookie, http.MethodPost, rsBase+"/onboarding/ack", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &ob)
	if rec.Code != http.StatusOK || !ob.Reviewed {
		t.Fatalf("ack = %d %+v", rec.Code, ob)
	}
	if got := guideState(); got != "done" {
		t.Fatalf("step after ack = %s", got)
	}
}

func TestRuntimeSettings_WebhookReplayToggle(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	rt.builder = &fakeBuilder{tag: "web:sha1"}
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyWebhookReplayProtection, json.RawMessage(`false`), "test"); err != nil {
		t.Fatal(err)
	}
	body := pushBody("refs/heads/main")
	sig := sign([]byte(created.WebhookSecret), body)
	requireStatusOK(t, postGitWebhook(t, rt, "X-Hub-Signature-256", sig, body, map[string]string{"X-GitHub-Delivery": "a"}))
	replay := postGitWebhook(t, rt, "X-Hub-Signature-256", sig, body, map[string]string{"X-GitHub-Delivery": "b"})
	if strings.Contains(replay.Body.String(), webhookNoteReplayedBody) {
		t.Fatal("replay guard still active after being turned off")
	}
}

func TestRuntimeSettings_SensitiveReadAuditToggle(t *testing.T) {
	rt, db := newTestRouter(t)
	_ = loginTestSession(t, rt, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/env/export", nil)
	if !rt.sensitiveReadAudited(req) {
		t.Fatal("sensitive reads must be audited by default")
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyAuditSensitiveReads, json.RawMessage(`false`), "test"); err != nil {
		t.Fatal(err)
	}
	if rt.sensitiveReadAudited(req) {
		t.Fatal("sensitive read still audited after being turned off")
	}
	if !sensitiveReadRoute(req) {
		t.Fatal("route classification must not depend on the setting")
	}
}

func TestLoopbackTrustFlag(t *testing.T) {
	loopbackDistrusted.Store(true)
	t.Cleanup(func() { loopbackDistrusted.Store(false) })
	if isTrustedPeer("127.0.0.1", nil) || peerTrusted(netipLoopback()) {
		t.Fatal("loopback trusted although the setting is off")
	}
	loopbackDistrusted.Store(false)
	if !isTrustedPeer("127.0.0.1", nil) || !peerTrusted(netipLoopback()) {
		t.Fatal("loopback must be trusted by default")
	}
}
