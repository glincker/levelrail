package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/buildrecord"
	"github.com/glincker/levelrail/internal/store"
)

const provPersonalImage = "reg.example/web@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// seedProvenance installs the image guard and a personal-builder record, with web-staging
// in a dev environment and web-prod in a production one.
func seedProvenance(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []store.Environment{
		{ID: "env_stg", ProjectID: "proj_1", Name: "staging", Kind: store.EnvironmentKindDev, CreatedAt: "2026-08-20T00:00:00Z"},
		{ID: "env_prod", ProjectID: "proj_1", Name: "production", Kind: store.EnvironmentKindProduction, CreatedAt: "2026-08-20T00:00:00Z"},
	} {
		if err := db.SaveEnvironment(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	for app, env := range map[string]string{"web-staging": "env_stg", "web-prod": "env_prod"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "levelrail/web:1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceProject(ctx, app, "proj_1"); err != nil {
			t.Fatal(err)
		}
		if err := db.SetServiceEnvironment(ctx, app, env); err != nil {
			t.Fatal(err)
		}
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer := buildrecord.NewSigner(priv)
	st := buildrecord.DBStore{DB: db}
	signed, err := signer.Sign(buildrecord.Claims{App: "web-staging", Environment: "staging", Repo: "reg.example/web:dev", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", BuilderNodeID: "node_p", BuilderTrust: buildrecord.TierPersonal, IssuedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRecord(ctx, "rec_1", "web-staging", signed); err != nil {
		t.Fatal(err)
	}
	gate := &buildrecord.Gate{Signer: signer, Store: st, Policy: func(ctx context.Context, app string) (buildrecord.Policy, error) {
		env, err := db.EnvironmentOfApp(ctx, app)
		if err != nil || env == nil {
			return buildrecord.Policy{}, err
		}
		return buildrecord.Policy{EnvKind: env.Kind, EnvName: env.Name}, nil
	}}
	db.SetDesiredImageGuard(gate.CheckImage)
}

func provPost(t *testing.T, rt *Router, db *store.DB, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedRequest(t, loginTestSession(t, rt, db), http.MethodPost, path, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func provImage(t *testing.T, db *store.DB, app string) string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	return svc.Image
}

func TestPromote_PersonalBuiltDigestRefusedForProduction(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProvenance(t, db)
	// Save the source's image as the personal digest before the guard sees it: staging allows it.
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web-staging", Image: provPersonalImage, Port: 3000}); err != nil {
		t.Fatalf("staging must accept the personal digest: %v", err)
	}
	rec := provPost(t, rt, db, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","force":true,"confirm":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if got := provImage(t, db, "web-prod"); got != "levelrail/web:1" {
		t.Fatalf("production image changed to %q", got)
	}
}

func TestRollback_PersonalBuiltDigestRefusedForProduction(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProvenance(t, db)
	rec := provPost(t, rt, db, "/api/v1/apps/web-prod/deploys", `{"image":"`+provPersonalImage+`","confirm":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	if got := provImage(t, db, "web-prod"); got != "levelrail/web:1" {
		t.Fatalf("production image changed to %q", got)
	}
}

func TestDeployImage_PersonalBuiltDigestAllowedForStaging(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProvenance(t, db)
	rec := provPost(t, rt, db, "/api/v1/apps/web-staging/deploys", `{"image":"`+provPersonalImage+`","confirm":true}`)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("staging refused the digest: %s", rec.Body.String())
	}
	if got := provImage(t, db, "web-staging"); got != provPersonalImage {
		t.Fatalf("staging image = %q, want the personal digest", got)
	}
}

func TestStoreGuard_NoSecretsManagerGateStillRefuses(t *testing.T) {
	_, db := newTestRouter(t)
	seedProvenance(t, db)
	// A gate with no signer must fail closed on a personal record rather than pass it.
	st := buildrecord.DBStore{DB: db}
	g := &buildrecord.Gate{Store: st, Policy: func(context.Context, string) (buildrecord.Policy, error) {
		return buildrecord.Policy{EnvKind: "custom"}, nil
	}}
	if err := g.CheckImage(context.Background(), "web-staging", provPersonalImage); err == nil {
		t.Fatal("unsigned gate passed a personal record")
	}
	prod := &buildrecord.Gate{Store: st, Policy: func(context.Context, string) (buildrecord.Policy, error) {
		return buildrecord.Policy{EnvKind: "production"}, nil
	}}
	if err := prod.CheckImage(context.Background(), "web-prod", provPersonalImage); err == nil {
		t.Fatal("production accepted a personal builder")
	}
	if err := prod.CheckImage(context.Background(), "web-prod", "reg.example/other@sha256:zzzz"); err != nil {
		t.Fatalf("image without a record must pass: %v", err)
	}
}

func TestBuildRecord_NotFoundForTokenScopedToAnotherApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProvenance(t, db)
	if err := db.LinkBuildRecordAttempt(context.Background(), "rec_1", "build_1"); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "rec-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-staging", "Deny", "*", "app:web-staging", store.PrincipalTypeToken, "rec-tok")
	get := func(mutate func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/builds/build_1/record", nil)
		mutate(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(bearer(tok)); code != http.StatusNotFound {
		t.Fatalf("scoped token status = %d, want 404", code)
	}
	cookie := loginTestSession(t, rt, db)
	if code := get(func(r *http.Request) { r.AddCookie(cookie) }); code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200", code)
	}
}
