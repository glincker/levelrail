package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A retained deploy copy shares its app's AppID; it must not make the app's CI
// token look like a caller of a second, unreadable service.
func TestCITokenStillReachesAppWithRetainedDeployCopy(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SetDashboardURL(ctx, "https://cp.example.com"); err != nil {
		t.Fatal(err)
	}
	rt.SetCIIDTokenVerifier(&fakeCIVerifier{claims: CIClaims{Repository: "acme/site", Ref: "refs/heads/main", Subject: "repo:acme/site"}})
	for _, name := range []string{"web", "other"} {
		if err := db.SaveApp(ctx, store.App{ID: name, Name: name}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 3000, AppID: name}); err != nil {
			t.Fatal(err)
		}
	}
	copySvc := store.DesiredService{Name: "web-dep1abcd", Image: "img:v0", Port: 3000, AppID: "web"}
	if err := db.SaveDesiredService(ctx, copySvc); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployURL(ctx, store.DeployURL{ServiceName: copySvc.Name, AppName: "web", AttemptID: "a1", Host: "web-dep1abcd.example.com"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", `{"repository":"acme/site","ref_pattern":"refs/heads/main"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create trust = %d %s", rec.Code, rec.Body)
	}
	rec = ciExchange(t, rt, "web")
	var out ciExchangeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("exchange = %d %s", rec.Code, rec.Body)
	}

	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+out.Token)
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, req)
		return w.Code
	}
	for _, path := range []string{"/api/v1/apps/web/deploy-attempts", "/api/v1/apps/web/deploys"} {
		if code := call(http.MethodGet, path); code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, code)
		}
	}
	// Rollback resolves the app before it looks for the attempt, so the app 404 is the regression signal.
	if code := call(http.MethodPost, "/api/v1/apps/web/deploys/missing/rollback"); code == http.StatusNotFound {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/deploys/missing/rollback", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+out.Token)
		rt.Handler().ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "app not found") {
			t.Fatalf("rollback hid the app: %s", rec.Body)
		}
	}
	// The scope stays one app: neither the copy nor another app opens up.
	for _, path := range []string{"/api/v1/apps/other/deploys", "/api/v1/apps/web-dep1abcd/deploys"} {
		if code := call(http.MethodGet, path); code == http.StatusOK {
			t.Fatalf("GET %s = 200, token reached beyond its app", path)
		}
	}
}
