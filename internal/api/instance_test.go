package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedBoundTestToken(t *testing.T, db *store.DB, id, plaintext string, abilities ...string) {
	t.Helper()
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: id, TokenHash: hashToken(plaintext), Abilities: abilities, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

func bearerGet(rt *Router, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestInstanceEndpoint_PublicIdentityNoSecrets(t *testing.T) {
	rt, db := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got instanceResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	info, err := db.GetInstanceInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != info.InstanceID || got.Fingerprint != info.Fingerprint || got.Fingerprint == "" {
		t.Errorf("response %+v does not match store identity %+v", got, info)
	}
	if strings.Contains(rec.Body.String(), "key") {
		t.Errorf("response must not carry key material: %s", rec.Body.String())
	}
}

func TestTokenBinding_BindsOnFirstUseThenRejectsOtherInstance(t *testing.T) {
	rt, db := newTestRouter(t)
	seedBoundTestToken(t, db, "tok_bind", "plain-bind-token", AbilityRead)

	if rec := bearerGet(rt, "/api/v1/apps", "plain-bind-token"); rec.Code != http.StatusOK {
		t.Fatalf("first use: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	bindings, err := db.ListTokenBindings(context.Background())
	if err != nil || bindings["tok_bind"].InstanceID == "" {
		t.Fatalf("token was not bound on first use: %v %+v", err, bindings)
	}

	if _, err := db.ExecContext(context.Background(), `UPDATE api_token_bindings SET instance_id = 'inst_other' WHERE token_id = 'tok_bind'`); err != nil {
		t.Fatal(err)
	}
	rec := bearerGet(rt, "/api/v1/apps", "plain-bind-token")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "bound to another server") {
		t.Fatalf("cloned token: status = %d, body = %s, want 401 bound to another server", rec.Code, rec.Body.String())
	}
}

func TestInstanceMoved_RejectsBoundTokensUntilConfirmed(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedBoundTestToken(t, db, "tok_move", "plain-move-token", AbilityRead)
	ctx := context.Background()

	dir := t.TempDir()
	if moved, err := db.ReconcileInstanceHost(ctx, dir); err != nil || moved {
		t.Fatalf("first reconcile moved=%v err=%v, want no move", moved, err)
	}
	if rec := bearerGet(rt, "/api/v1/apps", "plain-move-token"); rec.Code != http.StatusOK {
		t.Fatalf("pre-move use: status = %d", rec.Code)
	}

	moved, err := db.ReconcileInstanceHost(ctx, t.TempDir())
	if err != nil || !moved {
		t.Fatalf("restore into a fresh data dir: moved=%v err=%v, want moved", moved, err)
	}
	rt.invalidateInstanceInfo()
	if rec := bearerGet(rt, "/api/v1/apps", "plain-move-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-move use: status = %d, want 401", rec.Code)
	}

	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/instance/confirm-same", `{"confirm":"yes"}`)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong phrase: status = %d, want 400", rec.Code)
	}
	req = authedRequest(t, cookie, http.MethodPost, "/api/v1/instance/confirm-same", `{"confirm":"same instance"}`)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("confirm: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := bearerGet(rt, "/api/v1/apps", "plain-move-token"); rec.Code != http.StatusOK {
		t.Fatalf("after confirm: status = %d, want 200", rec.Code)
	}
}

func TestConfirmSameInstance_RequiresRootToken(t *testing.T) {
	rt, db := newTestRouter(t)
	seedBoundTestToken(t, db, "tok_ws", "plain-ws-token", AbilityWriteSensitive)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instance/confirm-same", strings.NewReader(`{"confirm":"same instance"}`))
	req.Header.Set("Authorization", "Bearer plain-ws-token")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a non-root token", rec.Code)
	}
}

func TestTokenList_ShowsBindingAndLastIP(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedBoundTestToken(t, db, "tok_list", "plain-list-token", AbilityRead)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer plain-list-token")
	req.RemoteAddr = "203.0.113.9:4444"
	rt.Handler().ServeHTTP(httptest.NewRecorder(), req)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", ""))
	var list []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	for _, tok := range list {
		if tok.ID == "tok_list" {
			if tok.InstanceID == "" || tok.LastIP == "" {
				t.Fatalf("token resource lacks binding: %+v", tok)
			}
			return
		}
	}
	t.Fatal("seeded token missing from list")
}

func TestArchiveList_AllowedAtWriteSensitive(t *testing.T) {
	rt, db := newTestRouter(t)
	seedBoundTestToken(t, db, "tok_arch", "plain-arch-token", AbilityWriteSensitive)
	if rec := bearerGet(rt, "/api/v1/archives", "plain-arch-token"); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("archive list with write:sensitive: status = %d, want it allowed", rec.Code)
	}
	seedBoundTestToken(t, db, "tok_arch_r", "plain-arch-read", AbilityRead)
	if rec := bearerGet(rt, "/api/v1/archives", "plain-arch-read"); rec.Code != http.StatusForbidden {
		t.Fatalf("archive list with read only: status = %d, want 403", rec.Code)
	}
}
