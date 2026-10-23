package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

func auditActions(t *testing.T, db *store.DB) []string {
	t.Helper()
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Action)
	}
	return out
}

func TestRotateSecretValueNeverReturnsOldValueAndRecordsAudit(t *testing.T) {
	t.Setenv(secretrotation.EnvGraceHours, "6")
	rt, db, mgr, cookie := newRotationRouter(t)
	ctx := context.Background()
	path := "/api/v1/apps/web/secrets/JWT_SECRET/rotate"

	if rec := rotationDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/secrets/NOPE/rotate", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing secret = %d, want 404", rec.Code)
	}
	if rec := rotationDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/secrets/JWT_SECRET_PREVIOUS/rotate", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("grace key = %d, want 400", rec.Code)
	}

	rec := rotationDo(t, rt, cookie, http.MethodPost, path, `{"value":"brand-new-value"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "original-value") || strings.Contains(body, "brand-new-value") {
		t.Fatal("response leaked a secret value")
	}
	if !strings.Contains(body, `"last_rotated_at"`) || !strings.Contains(body, `"previous_expires_at"`) {
		t.Fatalf("response lacks rotation timestamps: %s", body)
	}
	if v, _ := mgr.Resolve(ctx, "web", "JWT_SECRET"); v != "brand-new-value" {
		t.Fatal("new value not stored")
	}
	if prev, _ := mgr.Resolve(ctx, "web", "JWT_SECRET_PREVIOUS"); prev != "original-value" {
		t.Fatal("previous value not kept for the grace window")
	}

	list := rotationDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/secret-rotation", "")
	if strings.Contains(list.Body.String(), "original-value") || strings.Contains(list.Body.String(), "brand-new-value") {
		t.Fatal("rotation list leaked a secret value")
	}

	found := false
	for _, a := range auditActions(t, db) {
		if a == secretrotation.ActionRotate {
			found = true
		}
	}
	if !found {
		t.Fatal("rotation was not audited")
	}
}

func TestRotateVaultCredentialKeepsPreviousForGraceAndHidesValues(t *testing.T) {
	t.Setenv(secretrotation.EnvGraceHours, "6")
	fake := newFakeVaultSecrets()
	fake.values[store.VaultCredentialEnvKey] = "old-token-value"
	rt, db := newTestRouterWithVaultSecrets(t, fake)
	cookie := loginTestSession(t, rt, db)
	path := "/api/v1/settings/vault/rotate-credential"

	if rec := rotationDo(t, rt, cookie, http.MethodPost, path, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty credential = %d, want 400", rec.Code)
	}
	rec := rotationDo(t, rt, cookie, http.MethodPost, path, `{"credential":"new-token-value"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "old-token-value") || strings.Contains(rec.Body.String(), "new-token-value") {
		t.Fatal("response leaked a credential")
	}
	if !strings.Contains(rec.Body.String(), `"credential_rotated_at"`) || !strings.Contains(rec.Body.String(), `"previous_expires_at"`) {
		t.Fatalf("response lacks rotation timestamps: %s", rec.Body.String())
	}
	if fake.values[store.VaultCredentialEnvKey] != "new-token-value" || fake.values[vaultPreviousKey] != "old-token-value" {
		t.Fatal("credential or previous credential not stored")
	}
	found := false
	for _, a := range auditActions(t, db) {
		if a == auditActionVaultRotate {
			found = true
		}
	}
	if !found {
		t.Fatal("vault rotation was not audited")
	}

	if err := db.SetVaultCredentialRotation(context.Background(), time.Now().Add(-7*time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	get := rotationDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/vault", "")
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), `"previous_expires_at"`) {
		t.Fatalf("expired grace still reported: %d %s", get.Code, get.Body.String())
	}
	if _, ok := fake.values[vaultPreviousKey]; ok {
		t.Fatal("previous credential kept past the grace window")
	}
}
