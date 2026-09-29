package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestHandleDeployServiceTemplateNow_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/service-templates/redis-cache-starter/deploy", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeployServiceTemplateNow_UnknownTemplate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/service-templates/does-not-exist/deploy", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleDeployServiceTemplateNow_Success uses "redis-cache-starter":
// a real catalog entry whose Compose body has no SERVICE_ magic var at
// all, so it's false in Router.templateRequiresConfig and the one-click
// path should deploy straight through.
func TestHandleDeployServiceTemplateNow_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/service-templates/redis-cache-starter/deploy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.AppID, "redis-cache-starter-") {
		t.Errorf("AppID = %q, want prefix %q", got.AppID, "redis-cache-starter-")
	}
	if len(got.Services) == 0 {
		t.Fatal("got 0 services, want at least 1")
	}

	if _, err := db.GetAppByName(context.Background(), got.AppID); err != nil {
		t.Errorf("GetAppByName(%q) error = %v, want the one-click deploy to have created it", got.AppID, err)
	}
}

// TestHandleDeployServiceTemplateNow_RepeatDeploysGetDistinctNames
// proves two one-click deploys of the same template land as two
// separate apps (generateOneClickAppName's collision check) rather than
// SaveApp's own upsert-by-ID silently overwriting the first one.
func TestHandleDeployServiceTemplateNow_RepeatDeploysGetDistinctNames(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	first := deployTemplateNow(t, rt, cookie, "redis-cache-starter")
	second := deployTemplateNow(t, rt, cookie, "redis-cache-starter")
	if first == second {
		t.Errorf("two one-click deploys of the same template both got app id %q, want distinct names", first)
	}
}

// TestHandleDeployServiceTemplateNow_RequiresConfiguration_Conflict uses
// "copyparty": a real catalog entry whose command: array embeds
// $SERVICE_PASSWORD_ADMIN. compose.ResolveMagicVars' own doc comment
// explains why that's unresolved even though PASSWORD is a generatable
// kind: Command has no secret-storage indirection to land a generated
// value in, so a generatable token found there is always reported as
// unresolved instead of generated, so it's true in
// Router.templateRequiresConfig for it: the one-click path must refuse
// rather than deploy with that admin password left as a literal,
// unset placeholder.
func TestHandleDeployServiceTemplateNow_RequiresConfiguration_Conflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/service-templates/copyparty/deploy", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	apps, err := db.ListApps(context.Background())
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("ListApps() = %d apps, want 0: a conflicting template must not create anything", len(apps))
	}
}

func TestComputeTemplateRequiresConfig(t *testing.T) {
	got := computeTemplateRequiresConfig()
	tests := []struct {
		id   string
		want bool
	}{
		{"redis-cache-starter", false}, // no SERVICE_ var at all
		{"copyparty", true},            // $SERVICE_PASSWORD_ADMIN inside command:, unresolved
	}
	for _, tt := range tests {
		if _, ok := catalog.TemplateByID(tt.id); !ok {
			t.Fatalf("template %q not found in catalog", tt.id)
		}
		if got[tt.id] != tt.want {
			t.Errorf("computeTemplateRequiresConfig()[%q] = %v, want %v", tt.id, got[tt.id], tt.want)
		}
	}
}

func deployTemplateNow(t *testing.T, rt *Router, cookie *http.Cookie, id string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/service-templates/"+id+"/deploy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got.AppID
}
