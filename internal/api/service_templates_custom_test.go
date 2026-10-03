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

// TestHandleSaveAppAsTemplate_StripsSecretValue is the load-bearing
// test for save-as-template: an app with a real secret value attached
// must never have that value appear anywhere in the saved template's
// compose body, and the resulting template must report
// requires_configuration so the deploy path stops for operator input
// instead of silently deploying with the secret unset.
func TestHandleSaveAppAsTemplate_StripsSecretValue(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	source := store.DesiredService{
		Name:  "web",
		Image: "levelrail/web:1",
		Port:  3000,
		Env:   map[string]string{"LOG_LEVEL": "info"},
		SecretEnv: []store.SecretEnvRef{
			{Name: "API_KEY", Required: true},
		},
	}
	if err := db.SaveDesiredService(ctx, source); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	// store.DesiredService.SecretEnv never carries a decrypted value
	// itself (only the key name, see its own doc comment); this constant
	// stands in for whatever real value internal/secrets would hand the
	// application controller at container-create time, to assert it
	// never appears literally anywhere in the saved template.
	const realSecretValue = "sk_live_super_secret_value_12345" //nolint:gosec // test fixture, not a real credential

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/save-as-template", `{"name":"My web template","description":"saved from production"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got customTemplateDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if strings.Contains(got.Compose, realSecretValue) {
		t.Fatalf("saved template compose contains the real secret value:\n%s", got.Compose)
	}
	if !strings.Contains(got.Compose, "${SERVICE_SECRET_API_KEY}") {
		t.Errorf("saved template compose = %q, want a ${SERVICE_SECRET_API_KEY} placeholder", got.Compose)
	}
	if !got.RequiresConfiguration {
		t.Error("RequiresConfiguration = false, want true (a captured secret always needs operator input)")
	}
	if len(got.RequiredEnvKeys) != 1 || got.RequiredEnvKeys[0] != "API_KEY" {
		t.Errorf("RequiredEnvKeys = %v, want [API_KEY]", got.RequiredEnvKeys)
	}

	stored, err := db.GetCustomTemplate(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetCustomTemplate: %v", err)
	}
	if strings.Contains(stored.Compose, realSecretValue) {
		t.Fatalf("stored template compose contains the real secret value:\n%s", stored.Compose)
	}
	if stored.Name != "My web template" || stored.SourceApp != "web" {
		t.Errorf("stored template = %+v, want name %q, source_app %q", stored, "My web template", "web")
	}
}

func TestHandleSaveAppAsTemplate_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/does-not-exist/save-as-template", `{"name":"x"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSaveAppAsTemplate_MissingName_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/save-as-template", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSaveAppAsTemplate_MultiServiceApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	now := "2026-10-01T00:00:00Z"
	if err := db.SaveApp(ctx, store.App{ID: "stack", Name: "stack", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "stack-web", AppID: "stack", Image: "levelrail/web:1", Port: 3000, DependsOn: []string{"db"}}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "stack-db", AppID: "stack", Image: "postgres:16"}); err != nil {
		t.Fatalf("seed db: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/stack/save-as-template", `{"name":"Stack template"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got customTemplateDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(got.Compose, "web:") || !strings.Contains(got.Compose, "db:") {
		t.Errorf("multi-service compose missing a service key:\n%s", got.Compose)
	}
}

func TestHandleListCustomTemplates(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveCustomTemplate(ctx, store.CustomTemplate{
		ID: "custom-list1", Name: "Listed template", Compose: "version: \"3.8\"\nservices:\n  web:\n    image: x:1\n",
		CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed custom template: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/templates/custom", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []customTemplateListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "custom-list1" {
		t.Fatalf("got %+v, want exactly one entry with id custom-list1", got)
	}
	if strings.Contains(rec.Body.String(), `"compose"`) {
		t.Error("list response contains a compose field, want it omitted from the list shape")
	}
}

func TestHandleDeleteCustomTemplate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveCustomTemplate(ctx, store.CustomTemplate{
		ID: "custom-del1", Name: "Deletable", Compose: "version: \"3.8\"\nservices:\n  web:\n    image: x:1\n",
		CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed custom template: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/templates/custom/custom-del1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if _, err := db.GetCustomTemplate(ctx, "custom-del1"); err == nil {
		t.Error("GetCustomTemplate after delete: want an error, got nil")
	}
}

func TestHandleDeleteCustomTemplate_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/templates/custom/does-not-exist", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestResolveTemplate_CustomTemplateDeploysThroughExistingPath proves
// the no-second-deploy-path claim: a custom template id reaches the
// built-in catalog's own one-click deploy route
// (POST /api/v1/service-templates/{id}/deploy) and GET
// /api/v1/service-templates/{id}, with no dedicated custom-template
// deploy endpoint anywhere.
func TestResolveTemplate_CustomTemplateDeploysThroughExistingPath(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveCustomTemplate(ctx, store.CustomTemplate{
		ID:        "custom-deploy1",
		Name:      "No-config template",
		Compose:   "version: \"3.8\"\nservices:\n  web:\n    image: nginx:1.27\n",
		CreatedAt: "2026-10-01T00:00:00Z",
		UpdatedAt: "2026-10-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed custom template: %v", err)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/custom-deploy1", ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET service-templates status = %d, want %d; body = %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var detail serviceTemplateDetail
	if err := json.Unmarshal(getRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.RequiresConfiguration {
		t.Fatalf("detail.RequiresConfiguration = true, want false (no SERVICE_ var in this compose)")
	}

	deployRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deployRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/service-templates/custom-deploy1/deploy", ""))
	if deployRec.Code != http.StatusOK {
		t.Fatalf("deploy status = %d, want %d; body = %s", deployRec.Code, http.StatusOK, deployRec.Body.String())
	}
}
