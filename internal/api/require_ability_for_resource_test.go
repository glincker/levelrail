package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// attachTestPolicy creates a single-statement policy and attaches it to
// principalID, the shortest path from "I want this principal to have
// this Allow/Deny on this resource" to a saved, attached row, for tests
// that exercise requireAbilityForResource rather than the IAM CRUD API
// itself (iam_handlers_test.go already covers that layer directly).
func attachTestPolicy(t *testing.T, db *store.DB, name, effect, action, resource, principalType, principalID string) {
	t.Helper()
	ctx := context.Background()

	policyID, err := store.NewPolicyID()
	if err != nil {
		t.Fatalf("NewPolicyID() error = %v", err)
	}
	doc := `{"Statement":[{"Effect":"` + effect + `","Action":["` + action + `"],"Resource":["` + resource + `"]}]}`
	now := time.Now()
	if err := db.SavePolicy(ctx, store.Policy{
		ID: policyID, Name: name, Document: doc, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	attachmentID, err := store.NewPolicyAttachmentID()
	if err != nil {
		t.Fatalf("NewPolicyAttachmentID() error = %v", err)
	}
	if err := db.AttachPolicy(ctx, attachmentID, policyID, principalType, principalID); err != nil {
		t.Fatalf("AttachPolicy() error = %v", err)
	}
}

func TestRequireAbilityForResource_SessionDenyPolicyOverridesBaseAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "writer@example.com", []string{AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-write", "Deny", AbilityWrite, "app:prod-web", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/prod-web", ""))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("delete prod-web status = %d, want %d (explicit Deny must override base AbilityWrite), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/staging-web", ""))
	if allowedRec.Code != http.StatusNoContent {
		t.Fatalf("delete staging-web status = %d, want %d (Deny only scopes to app:prod-web), body = %s", allowedRec.Code, http.StatusNoContent, allowedRec.Body.String())
	}
}

func TestRequireAbilityForResource_SessionAllowPolicyGrantsBeyondBaseAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	attachTestPolicy(t, db, "allow-prod-write", "Allow", AbilityWrite, "app:prod-web", store.PrincipalTypeUser, reader.ID)
	cookie := sessionCookieForTest(t, rt, reader.ID)

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/prod-web", ""))
	if allowedRec.Code != http.StatusNoContent {
		t.Fatalf("delete prod-web status = %d, want %d (Allow policy grants write on this resource), body = %s", allowedRec.Code, http.StatusNoContent, allowedRec.Body.String())
	}

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/staging-web", ""))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("delete staging-web status = %d, want %d (Allow only scopes to app:prod-web, reader has no base write), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}
}

func TestRequireAbilityForResource_TokenDenyPolicyOverridesBaseAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}

	plaintext := "test-token-plaintext-value"
	tok := store.APIToken{ID: "tok_scoped", Name: "scoped", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now()}
	if err := db.SaveAPIToken(ctx, tok); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}
	attachTestPolicy(t, db, "deny-prod-write-token", "Deny", AbilityWrite, "app:prod-web", store.PrincipalTypeToken, tok.ID)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/prod-web", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequireAbilityForResource_NoPoliciesBehavesLikeRequireAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed web: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "plain-writer@example.com", []string{AbilityWrite})
	cookie := sessionCookieForTest(t, rt, writer.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (a principal with no attached policies must behave exactly like requireAbility)", rec.Code, http.StatusNoContent)
	}
}

func TestRequireAbilityForResource_DatabaseRouteWired(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "prod-main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed prod-main: %v", err)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "db-reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/prod-main", ""))
	if deniedRec.Code != http.StatusOK {
		t.Fatalf("status before policy = %d, want %d", deniedRec.Code, http.StatusOK)
	}

	attachTestPolicy(t, db, "deny-prod-db-read", "Deny", AbilityRead, "database:prod-main", store.PrincipalTypeUser, reader.ID)

	afterRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(afterRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/prod-main", ""))
	if afterRec.Code != http.StatusForbidden {
		t.Fatalf("status after Deny policy = %d, want %d, body = %s", afterRec.Code, http.StatusForbidden, afterRec.Body.String())
	}
}
