package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"gopkg.in/yaml.v3"
)

func TestHandleExportAppSpec_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/spec", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleExportAppSpec_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/spec", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleExportAppSpec_RoundTripsToValidYAML(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	seed := store.DesiredService{
		Name:      "web",
		Image:     "levelrail/web:1",
		Port:      3000,
		Domains:   []string{"web.example.com"},
		Env:       map[string]string{"LOG_LEVEL": "info"},
		SecretEnv: []string{"API_KEY"},
		Resources: &store.ServiceResources{MemoryBytes: 512 * 1024 * 1024, NanoCPUs: 500_000_000},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 5_000_000_000, Timeout: 2_000_000_000},
		},
		Labels: map[string]string{"team": "infra"},
	}
	if err := db.SaveDesiredService(context.Background(), seed); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/spec", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/yaml" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/yaml")
	}

	var got spec.Spec
	if err := yaml.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid YAML: %v; body = %s", err, rec.Body.String())
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	svc, ok := got.Services["web"]
	if !ok {
		t.Fatalf("Services[web] missing, got %+v", got.Services)
	}
	if svc.Port != 3000 {
		t.Errorf("Port = %d, want 3000", svc.Port)
	}
	if len(svc.Domains) != 1 || svc.Domains[0] != "web.example.com" {
		t.Errorf("Domains = %v, want [web.example.com]", svc.Domains)
	}
	if v, ok := svc.Env["LOG_LEVEL"]; !ok || v.Value != "info" || v.Secret {
		t.Errorf("Env[LOG_LEVEL] = %+v, want literal value %q", v, "info")
	}
	if v, ok := svc.Env["API_KEY"]; !ok || !v.Secret || v.Required {
		t.Errorf("Env[API_KEY] = %+v, want a secret marker, not required (known fidelity loss)", v)
	}
	if svc.Resources == nil || svc.Resources.Memory != "512Mi" || svc.Resources.CPU != 0.5 {
		t.Errorf("Resources = %+v, want Memory=512Mi CPU=0.5", svc.Resources)
	}
	if svc.Health == nil || svc.Health.Readiness == nil || svc.Health.Readiness.Path != "/healthz" {
		t.Errorf("Health = %+v, want Readiness.Path=/healthz", svc.Health)
	}
	if svc.Labels["team"] != "infra" {
		t.Errorf("Labels = %v, want team=infra", svc.Labels)
	}
	if svc.Build.Type != "" {
		t.Errorf("Build.Type = %q, want empty (build config is not tracked in desired state)", svc.Build.Type)
	}
}

func TestHandleExportAppSpec_RequiresReadAbility(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1"}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	const plaintext = "deploy-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/spec", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}
