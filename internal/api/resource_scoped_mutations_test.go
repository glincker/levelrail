package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// These tests prove the security-review fix itself: a Deny policy scoped
// to one app or database must actually block the newly-migrated mutating
// routes (deploy-like, destructive, tag, secrets, database), not just the
// original GET/PUT/DELETE trio requireAbilityForResource already covered.
// Each test denies one resource and confirms a sibling resource of the
// same kind is unaffected, the same shape as
// require_ability_for_resource_test.go's own tests.

func TestRequireAbilityForResource_RestartDenyBlocksOnlyThatApp(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	deployer := storeUserWithAbilitiesForTest(t, db, "deployer@example.com", []string{AbilityDeploy})
	attachTestPolicy(t, db, "deny-prod-restart", "Deny", AbilityDeploy, "app:prod-web", store.PrincipalTypeUser, deployer.ID)
	cookie := sessionCookieForTest(t, rt, deployer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/prod-web/restart", ""))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("restart prod-web status = %d, want %d (explicit Deny must block restart), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/staging-web/restart", ""))
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("restart staging-web status = %d, want %d (Deny only scopes to app:prod-web), body = %s", allowedRec.Code, http.StatusOK, allowedRec.Body.String())
	}
}

func TestRequireAbilityForResource_CloneDenyBlocksOnlyThatApp(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "writer-clone@example.com", []string{AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-clone", "Deny", AbilityWrite, "app:prod-web", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/prod-web/clone", `{"new_name":"prod-web-copy"}`))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("clone prod-web status = %d, want %d (explicit Deny must block clone), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/staging-web/clone", `{"new_name":"staging-web-copy"}`))
	if allowedRec.Code != http.StatusCreated {
		t.Fatalf("clone staging-web status = %d, want %d (Deny only scopes to app:prod-web), body = %s", allowedRec.Code, http.StatusCreated, allowedRec.Body.String())
	}
}

func TestRequireAbilityForResource_AttachTagDenyBlocksOnlyThatApp(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "writer-tag@example.com", []string{AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-tag", "Deny", AbilityWrite, "app:prod-web", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/prod-web/tags", `{"name":"env-prod"}`))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("tag prod-web status = %d, want %d (explicit Deny must block tag attach), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/staging-web/tags", `{"name":"env-prod"}`))
	if allowedRec.Code != http.StatusCreated {
		t.Fatalf("tag staging-web status = %d, want %d (Deny only scopes to app:prod-web), body = %s", allowedRec.Code, http.StatusCreated, allowedRec.Body.String())
	}
}

func TestRequireAbilityForResource_SetSecretDenyBlocksOnlyThatApp(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "prod-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed prod-web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "staging-web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed staging-web: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "writer-secret@example.com", []string{AbilityWriteSensitive})
	attachTestPolicy(t, db, "deny-prod-secret", "Deny", AbilityWriteSensitive, "app:prod-web", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/prod-web/secrets/API_KEY", `{"value":"sk-abc"}`))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("set secret on prod-web status = %d, want %d (explicit Deny must block secret write), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}
	if setter.calls != 0 {
		t.Errorf("SetValueGuarded called %d times, want 0: a denied request must never reach the secret setter", setter.calls)
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/staging-web/secrets/API_KEY", `{"value":"sk-abc"}`))
	if allowedRec.Code != http.StatusNoContent {
		t.Fatalf("set secret on staging-web status = %d, want %d (Deny only scopes to app:prod-web), body = %s", allowedRec.Code, http.StatusNoContent, allowedRec.Body.String())
	}
	if setter.calls != 1 {
		t.Errorf("SetValueGuarded called %d times, want 1 for the allowed request", setter.calls)
	}
}

func TestRequireAbilityForResource_DatabaseProjectDenyBlocksOnlyThatDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "prod-main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed prod-main: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "staging-main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed staging-main: %v", err)
	}

	writer := storeUserWithAbilitiesForTest(t, db, "writer-db@example.com", []string{AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-db-project", "Deny", AbilityWrite, "database:prod-main", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/prod-main/project", `{"project_id":""}`))
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("set project on prod-main status = %d, want %d (explicit Deny must block database mutation), body = %s", deniedRec.Code, http.StatusForbidden, deniedRec.Body.String())
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/staging-main/project", `{"project_id":""}`))
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("set project on staging-main status = %d, want %d (Deny only scopes to database:prod-main), body = %s", allowedRec.Code, http.StatusOK, allowedRec.Body.String())
	}
}
