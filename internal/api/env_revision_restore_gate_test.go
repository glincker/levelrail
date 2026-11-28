package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func restoreGateToken(t *testing.T, f envFixture, id string, abilities []string) string {
	t.Helper()
	plain := id + "-token-value" //nolint:gosec // fake fixture
	if err := f.db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return plain
}

func restoreGateSeed(t *testing.T, f envFixture) {
	t.Helper()
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3000","API_SECRET":"`+plainSecretValue+`"}}`, http.StatusOK)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3001"}}`, http.StatusOK)
}

func TestEnvRevisionRestore_ProtectedAppRefused(t *testing.T) {
	f := newEnvFixture(t)
	restoreGateSeed(t, f)
	if err := f.db.SetAppProtection(context.Background(), "web", "keep", "test"); err != nil {
		t.Fatal(err)
	}
	tok := restoreGateToken(t, f, "w1", []string{AbilityWrite, AbilityWriteSensitive})
	rec := f.serve(bearerRequest(http.MethodPost, "/api/v1/apps/web/env/revisions/1/restore", `{"confirm":"web"}`, tok))
	if rec.Code != http.StatusLocked {
		t.Fatalf("restore on protected app = %d, want 423: %s", rec.Code, rec.Body.String())
	}
}

func TestEnvRevisionRestore_SensitiveNeedsWriteSensitive(t *testing.T) {
	f := newEnvFixture(t)
	restoreGateSeed(t, f)
	tok := restoreGateToken(t, f, "w2", []string{AbilityWrite, AbilityReadSensitive})
	rec := f.serve(bearerRequest(http.MethodPost, "/api/v1/apps/web/env/revisions/1/restore", `{"confirm":"web"}`, tok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("write-only restore adding a sensitive key = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Env["PORT"] != "3001" {
		t.Fatalf("env changed despite refusal: %+v %v", svc, err)
	}
}
