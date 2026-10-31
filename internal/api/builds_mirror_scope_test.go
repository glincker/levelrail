package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCloneCredentialScopeEmptyWithoutCredential(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	token, scope := rt.cloneCredential(context.Background(), "web", "https://example.com/org/web.git", false)
	if token != "" || scope != "" {
		t.Fatalf("no credential: token=%q scope=%q", token, scope)
	}
}

func TestTriggerRepoAllowedForTeamScopedCaller(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("img:1", nil), nil)
	seedWebApp(t, db)
	seedGitSourceForSchedule(t, db)

	teamCtx := context.WithValue(context.Background(), teamScopeKey{}, &teamScope{roles: map[string]string{"t1": "admin"}})
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(teamCtx)
	if !rt.triggerRepoAllowed(req, "web", "https://example.com/org/web") {
		t.Fatal("stored repo must be allowed")
	}
	if rt.triggerRepoAllowed(req, "web", "https://example.com/org/other.git") {
		t.Fatal("a different repo must be refused for a team-scoped caller")
	}
	instance := httptest.NewRequest(http.MethodPost, "/", nil)
	if !rt.triggerRepoAllowed(instance, "web", "https://example.com/org/other.git") {
		t.Fatal("instance-level caller keeps the existing behavior")
	}
}

func TestTriggerBuildFromCacheNeedsReadSensitive(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("img:1", nil), nil)
	seedWebApp(t, db)
	plain := "deploy-only-token-for-test"
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "dep", Name: "dep", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityDeploy}}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/builds", strings.NewReader(`{"repo_url":"https://example.com/org/web.git","ref":"main","from_cache":true}`))
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
