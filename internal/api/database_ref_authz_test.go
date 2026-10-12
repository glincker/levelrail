package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A writer on one app must not attach a database an IAM Deny hides from them.
func TestDatabaseRefs_DenyOnDatabaseBlocksAttach(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "x:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	for _, n := range []string{"prod-main", "open-main"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: n, Engine: store.EnginePostgres}); err != nil {
			t.Fatalf("seed database %s: %v", n, err)
		}
	}
	writer := storeUserWithAbilitiesForTest(t, db, "writer@example.com", []string{AbilityRead, AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-db", "Deny", "*", "database:prod-main", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	cases := []struct {
		name, method, path, body string
	}{
		{"connections", http.MethodPost, "/api/v1/apps/web/connections", `{"database":"prod-main"}`},
		{"set database", http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"prod-main"}`},
	}
	for _, c := range cases {
		t.Run(c.name+" denied", func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, c.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (indistinguishable from an unknown database), body = %s", rec.Code, rec.Body.String())
			}
		})
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if len(svc.DatabaseEnv) != 0 || svc.DatabaseAttachment != nil {
		t.Fatalf("denied database was persisted: env=%+v att=%+v", svc.DatabaseEnv, svc.DatabaseAttachment)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"open-main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed database status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseRefs_DeploySpecFromRefDenied(t *testing.T) {
	rt, db := newTestRouterWithBuilderAndSecrets(t, &fakeBuilder{tag: "img:sha"}, newFakeFetch("/tmp/checkout", nil), &fakeSecretSetter{})
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "x:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "prod-main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	user := storeUserWithAbilitiesForTest(t, db, "dev@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	attachTestPolicy(t, db, "deny-prod-db", "Deny", "*", "database:prod-main", store.PrincipalTypeUser, user.ID)
	cookie := sessionCookieForTest(t, rt, user.ID)

	body := `{"repo_url":"https://example.com/r.git","ref":"main","services":{"web":{"build":{"type":"dockerfile","path":"./Dockerfile"},"port":3000,"env":{"DATABASE_URL":{"from":"postgres.prod-main.url"}}}}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-spec", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}
