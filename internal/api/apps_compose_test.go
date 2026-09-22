package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const validComposeYAML = `
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
  redis:
    image: redis:7
`

const webOnlyComposeYAML = `
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
`

const bindMountComposeYAML = `
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    volumes:
      - /srv/myapp/data:/data
`

func TestHandleDeployCompose_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/myapp/compose", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleDeployCompose_BindMount_PlainDeployToken_Forbidden proves a
// compose file bind-mounting a host directory needs AbilityRoot on top
// of this route's own AbilityDeploy gate: a token scoped to nothing but
// AbilityDeploy (this route's own gate, so it can deploy an ordinary
// compose file fine) must still be rejected here, and must not have
// created anything.
func TestHandleDeployCompose_BindMount_PlainDeployToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "deploy-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/myapp/compose", strings.NewReader(bindMountComposeYAML))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain deploy token must not be able to bind-mount a host directory", rec.Code, http.StatusForbidden)
	}

	if _, err := db.GetAppByName(ctx, "myapp"); err == nil {
		t.Error("GetAppByName() error = nil, want the app to not have been created")
	}
}

// TestHandleDeployCompose_BindMount_RootCaller_Succeeds is the positive
// counterpart: a root-ability caller (loginTestSession's own bootstrapped
// admin) can deploy the exact same bind-mounting compose file that
// TestHandleDeployCompose_BindMount_PlainDeployToken_Forbidden rejects.
func TestHandleDeployCompose_BindMount_RootCaller_Succeeds(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", bindMountComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(got.Services))
	}

	svc, err := db.GetDesiredService(context.Background(), got.Services[0].Name)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	wantBindMounts := []store.ServiceBindMount{{HostPath: "/srv/myapp/data", ContainerPath: "/data"}}
	if len(svc.BindMounts) != 1 || svc.BindMounts[0] != wantBindMounts[0] {
		t.Errorf("BindMounts = %+v, want %+v", svc.BindMounts, wantBindMounts)
	}
}

func TestHandleDeployCompose_CreatesAppAndServices(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AppID != "myapp" {
		t.Errorf("AppID = %q, want myapp", got.AppID)
	}
	if len(got.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(got.Services))
	}

	app, err := db.GetAppByName(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("GetAppByName() error = %v", err)
	}
	services, err := db.ListServicesByApp(context.Background(), app.ID)
	if err != nil {
		t.Fatalf("ListServicesByApp() error = %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("ListServicesByApp() returned %d services, want 2", len(services))
	}
}

// TestHandleDeployCompose_PullPolicyPropagates checks that a service's
// pull_policy: always makes it all the way through the HTTP response
// and into the persisted store.DesiredService, and that a sibling
// service with no pull_policy: declared stays empty.
func TestHandleDeployCompose_PullPolicyPropagates(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	const composeYAML = `
services:
  web:
    image: nginx:latest
    ports: ["8080:80"]
    pull_policy: always
  redis:
    image: redis:7
`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", composeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byName := make(map[string]appResource, len(got.Services))
	for _, svc := range got.Services {
		byName[svc.Name] = svc
	}
	if pp := byName["myapp-web"].PullPolicy; pp != store.PullPolicyAlways {
		t.Errorf("myapp-web PullPolicy = %q, want %q", pp, store.PullPolicyAlways)
	}
	if pp := byName["myapp-redis"].PullPolicy; pp != "" {
		t.Errorf("myapp-redis PullPolicy = %q, want empty", pp)
	}

	saved, err := db.GetDesiredService(context.Background(), "myapp-web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.PullPolicy != store.PullPolicyAlways {
		t.Errorf("persisted PullPolicy = %q, want %q", saved.PullPolicy, store.PullPolicyAlways)
	}
}

// TestHandleDeployCompose_RecordsDeployAttempts covers the gap found via
// live testing (deploying a template such as "Homepage" showed the
// reconciler's own real rollout, but the Deploys tab stayed "No deploys
// yet"): each service a compose file fans out into must get its own
// deploy_attempts row, source compose, the same way a plain image
// create/update already does under source image.
func TestHandleDeployCompose_RecordsDeployAttempts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	for _, tt := range []struct {
		service string
		image   string
	}{
		{service: "myapp-web", image: "nginx:1.27"},
		{service: "myapp-redis", image: "redis:7"},
	} {
		attempts, err := db.ListDeployAttempts(ctx, tt.service)
		if err != nil {
			t.Fatalf("ListDeployAttempts(%q) error = %v", tt.service, err)
		}
		if len(attempts) != 1 {
			t.Fatalf("service %q: got %d deploy attempts, want 1", tt.service, len(attempts))
		}
		if attempts[0].Image != tt.image {
			t.Errorf("service %q: Image = %q, want %q", tt.service, attempts[0].Image, tt.image)
		}
		if attempts[0].Source != store.DeployAttemptSourceCompose {
			t.Errorf("service %q: Source = %q, want %q", tt.service, attempts[0].Source, store.DeployAttemptSourceCompose)
		}
		if attempts[0].Status != store.DeployAttemptStatusSucceeded {
			t.Errorf("service %q: Status = %q, want %q", tt.service, attempts[0].Status, store.DeployAttemptStatusSucceeded)
		}
	}
}

func TestHandleDeployCompose_InvalidYAML(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", "not: [valid"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_RejectsBuild(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", "services:\n  web:\n    build: .\n"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_Redeploy_ReusesSameApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	apps, err := db.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps after redeploy, want 1 (must reuse, not duplicate)", len(apps))
	}
}

// TestHandleDeployCompose_Redeploy_PrunesRemovedService covers the real
// gap that motivated pruneStaleComposeServices: a redeploy dropping a
// service (here "redis") from the compose file must delete that
// service's own DesiredService row, not just leave it (and its
// containers) running forever alongside the services the new file still
// declares.
func TestHandleDeployCompose_Redeploy_PrunesRemovedService(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("first deploy: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "myapp-redis"); err != nil {
		t.Fatalf("myapp-redis GetDesiredService() error = %v, want it to exist after the first deploy", err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", webOnlyComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("redeploy: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := db.GetDesiredService(context.Background(), "myapp-redis"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Errorf("myapp-redis GetDesiredService() error = %v, want ErrServiceNotFound after redeploy dropped it", err)
	}
	if _, err := db.GetDesiredService(context.Background(), "myapp-web"); err != nil {
		t.Errorf("myapp-web GetDesiredService() error = %v, want it to still exist", err)
	}
}

const magicVarComposeYAML = `
services:
  app:
    image: myapp:latest
    environment:
      DB_PASSWORD: $SERVICE_PASSWORD_DB
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
`

func TestHandleDeployCompose_MagicVar_NoSecretsManagerConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_MagicVar_GeneratedAndSharedAcrossServices(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithComposeSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	appVal, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve myapp-app/DB_PASSWORD: %v", err)
	}
	dbVal, err := secretsManager.Resolve(context.Background(), "myapp-db", "POSTGRES_PASSWORD")
	if err != nil {
		t.Fatalf("resolve myapp-db/POSTGRES_PASSWORD: %v", err)
	}
	if appVal != dbVal {
		t.Errorf("app's DB_PASSWORD (%q) != db's POSTGRES_PASSWORD (%q), want the same shared generated value", appVal, dbVal)
	}
	if appVal == "" {
		t.Error("generated value is empty")
	}

	for _, svc := range got.Services {
		if _, ok := svc.Env["DB_PASSWORD"]; ok {
			t.Errorf("service %q: DB_PASSWORD leaked into literal Env, want it secret-only", svc.Name)
		}
	}
}

func TestHandleDeployCompose_MagicVar_RedeployReusesSameGeneratedValue(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithComposeSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	val1, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", magicVarComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	val2, err := secretsManager.Resolve(context.Background(), "myapp-app", "DB_PASSWORD")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if val1 != val2 {
		t.Errorf("redeploy regenerated the secret (%q != %q), want the same value reused across redeploys", val1, val2)
	}
}

func TestHandleDeployCompose_UnresolvedVar(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	yaml := "services:\n  app:\n    image: myapp:latest\n    environment:\n      API_KEY: ${SERVICE_OPENAI_API_KEY}\n"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", yaml))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDeployCompose_DomainAssignedFromExtension(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	yaml := "x-levelrail-domains:\n  app: app.example.com\nservices:\n  app:\n    image: myapp:latest\n    environment:\n      FRONTEND_URL: ${SERVICE_FQDN_APP}\n"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", yaml))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(got.Services))
	}
	svc := got.Services[0]
	if len(svc.Domains) != 1 || svc.Domains[0] != "app.example.com" {
		t.Errorf("Domains = %+v, want [app.example.com]", svc.Domains)
	}
	if svc.Env["FRONTEND_URL"] != "https://app.example.com" {
		t.Errorf("Env[FRONTEND_URL] = %q, want https://app.example.com", svc.Env["FRONTEND_URL"])
	}
}

func TestHandleDeployCompose_NoNoticesForPlainFile(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", validComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Notices) != 0 {
		t.Errorf("Notices = %+v, want none", got.Notices)
	}
}

func TestHandleDeployCompose_RestartAndNetworksSurfaceAsNotices(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	yaml := "services:\n" +
		"  web:\n" +
		"    image: nginx:1.27\n" +
		"    restart: always\n" +
		"    networks: [frontend]\n"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", yaml))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Notices) != 2 {
		t.Fatalf("Notices = %+v, want 2 entries (one restart note, one networks warning)", got.Notices)
	}
	if got.Notices[0].Level != "note" {
		t.Errorf("Notices[0].Level = %q, want note", got.Notices[0].Level)
	}
	if got.Notices[1].Level != "warning" {
		t.Errorf("Notices[1].Level = %q, want warning", got.Notices[1].Level)
	}
}

// TestHandleDeployCompose_LongFormPortsAndVolumes proves a real-world
// compose file using Compose's long mapping form for ports: and
// volumes: (common in upstream project compose files, unlike this
// platform's own short-form-only templates) deploys end to end, the
// same as validComposeYAML's short-form equivalent.
func TestHandleDeployCompose_LongFormPortsAndVolumes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	const yaml = `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
    volumes:
      - type: volume
        source: web-data
        target: /usr/share/nginx/html
        read_only: true
`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/compose", yaml))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(got.Services))
	}

	svc, err := db.GetDesiredService(context.Background(), got.Services[0].Name)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Port != 80 {
		t.Errorf("Port = %d, want 80 (from long-form target:)", svc.Port)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0].ContainerPath != "/usr/share/nginx/html" {
		t.Errorf("Volumes = %+v, want one entry mounted at /usr/share/nginx/html", svc.Volumes)
	}
}
