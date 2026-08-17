package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleCreateToken_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"ci token","abilities":["read","deploy"]}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Token == "" {
		t.Error("Token is empty, want a real plaintext token")
	}
	if got.Name != "ci token" {
		t.Errorf("Name = %q, want %q", got.Name, "ci token")
	}
	if len(got.Abilities) != 2 {
		t.Errorf("Abilities = %v, want 2 entries", got.Abilities)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}

	// Independent verification: the stored row's hash matches the
	// returned plaintext, not just that the handler claims success.
	rec2, err := db.GetAPITokenByHash(context.Background(), hashToken(got.Token))
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if rec2.ID != got.ID {
		t.Errorf("stored token ID = %q, want %q", rec2.ID, got.ID)
	}
}

func TestHandleCreateToken_NeverExpiresByDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"x","abilities":["read"]}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var got createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil (no expires_in_days sent)", got.ExpiresAt)
	}
}

func TestHandleCreateToken_InvalidAbilities(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	tests := []string{
		`{"name":"x","abilities":[]}`,
		`{"name":"x","abilities":["root","read"]}`,
		`{"name":"x","abilities":["not-a-real-ability"]}`,
		`{"name":"","abilities":["read"]}`,
		`{"name":"x","abilities":["read"],"expires_in_days":-1}`,
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateToken_MalformedBody(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateToken_RequiresSession_TokenCannotMintToken(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	// Seed a root-scoped bearer token directly in the store, then try
	// to use it to mint a new token: this must fail, tokens can never
	// act on the token set themselves, only an interactive session can.
	const plaintext = "root-token-plaintext" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_root", Name: "root", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(`{"name":"x","abilities":["read"]}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: a bearer token, even root-scoped, must not be able to mint another token", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleListTokens(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_1", Name: "one", TokenHash: "h1", Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if strings.Contains(body, "h1") {
		t.Error("response contains the raw token hash, must never be exposed")
	}
	var got []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "tok_1" {
		t.Errorf("got %+v, want one token tok_1", got)
	}
}

func TestHandleRevokeToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_rev", Name: "x", TokenHash: "h2", Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/auth/tokens/tok_rev", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	got, err := db.GetAPITokenByHash(ctx, "h2")
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("token not revoked after DELETE")
	}
}

func TestHandleRevokeToken_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/auth/tokens/ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestTokenRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	tests := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/auth/tokens"},
		{http.MethodGet, "/api/v1/auth/tokens"},
		{http.MethodDelete, "/api/v1/auth/tokens/tok_x"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// --- requireAbility, exercised through a real gated route (GET /apps) ---

// TestRequireAbility_RootSessionCanAccessAnything replaces the old
// TestRequireAbility_SessionGrantsAccessRegardlessOfAbility: a session no
// longer bypasses the ability check by virtue of being a session, it
// passes here because loginTestSession's admin is bootstrapped with
// AbilityRoot (BootstrapAdmin's own doc comment), the same as a
// root-scoped bearer token. Exercised against both an AbilityRead route
// and an AbilityRoot route, so this isn't just AbilityRead getting lucky.
func TestRequireAbility_RootSessionCanAccessAnything(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /apps status = %d, want %d", rec.Code, http.StatusOK)
	}

	victim := storeUserForTest(t, db, "victim@example.com")
	recRoot := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recRoot, authedRequest(t, cookie, http.MethodDelete, "/api/v1/users/"+victim.ID, ""))
	if recRoot.Code != http.StatusNoContent {
		t.Fatalf("DELETE /users/{id} status = %d, want %d, body = %s", recRoot.Code, http.StatusNoContent, recRoot.Body.String())
	}
}

// TestRequireAbility_NonRootSessionWithoutRequiredAbility_Forbidden
// proves a session is now checked against its own user's Abilities, not
// treated as implicitly root: a session whose user holds only
// AbilityRead gets 403 on AbilityWrite/AbilityDeploy/AbilityRoot routes.
func TestRequireAbility_NonRootSessionWithoutRequiredAbility_Forbidden(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"AbilityWrite route (create app)", http.MethodPost, "/api/v1/apps", `{"name":"web","image":"levelrail/web:1","port":3000}`},
		{"AbilityDeploy route (trigger deploy)", http.MethodPost, "/api/v1/apps/web/deploys", `{}`},
		{"AbilityRoot route (delete user)", http.MethodDelete, "/api/v1/users/user_someone", ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)
			reader := storeUserWithAbilitiesForTest(t, db, fmt.Sprintf("reader-%d@example.com", i), []string{AbilityRead})
			cookie := sessionCookieForTest(t, rt, reader.ID)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.path, tt.body))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

// TestRequireAbility_SessionWithExactRequiredAbility_Succeeds proves a
// session holding exactly the required ability (not root) is let
// through, the session-branch equivalent of
// TestRequireAbility_BearerTokenWithMatchingAbility above.
func TestRequireAbility_SessionWithExactRequiredAbility_Succeeds(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	writer := storeUserWithAbilitiesForTest(t, db, "writer@example.com", []string{AbilityWrite})
	cookie := sessionCookieForTest(t, rt, writer.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"levelrail/web:1","port":3000}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestRequireAbility_BearerTokenWithMatchingAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "read-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := db.GetAPITokenByHash(ctx, hashToken(plaintext))
	if err != nil {
		t.Fatalf("GetAPITokenByHash() error = %v", err)
	}
	if updated.LastUsedAt == nil {
		t.Error("LastUsedAt not updated after a successful authenticated call")
	}
}

func TestRequireAbility_BearerTokenWithoutRequiredAbility_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "deploy-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// GET /apps requires AbilityRead; this token only has AbilityDeploy.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: a deploy-only token must not be able to read apps", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAbility_RootTokenCanAccessAnything(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "root-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_root2", Name: "root", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireAbility_RevokedTokenRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "revoked-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_revoked", Name: "x", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.RevokeAPIToken(ctx, "tok_revoked"); err != nil {
		t.Fatalf("RevokeAPIToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAbility_ExpiredTokenRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "expired-token" //nolint:gosec // fake fixture, not a real credential
	past := time.Now().Add(-1 * time.Hour)
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_expired", Name: "x", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot},
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAbility_UnknownTokenRejected(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer totally-made-up-token")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAbility_NoCredentialAtAll(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestBearerToken_MalformedHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "no Bearer prefix", header: "totally-made-up-token"},
		{name: "empty after Bearer", header: "Bearer "},
		{name: "wrong scheme", header: "Basic dXNlcjpwYXNz"},
		{name: "empty header", header: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if _, ok := bearerToken(req); ok {
				t.Errorf("bearerToken() ok = true for header %q, want false", tt.header)
			}
		})
	}
}

func TestBearerToken_ValidHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-real-token")
	got, ok := bearerToken(req)
	if !ok || got != "my-real-token" {
		t.Errorf("bearerToken() = (%q, %v), want (my-real-token, true)", got, ok)
	}
}
