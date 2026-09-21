package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppEgressPolicyRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/egress-policy"},
		{http.MethodPut, "/api/v1/apps/web/egress-policy"},
		{http.MethodDelete, "/api/v1/apps/web/egress-policy"},
	})
}

func TestHandleGetAppEgressPolicy_Unconfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/egress-policy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got egressPolicyResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AppName != "web" || got.Mode != "" || len(got.Allow) != 0 {
		t.Errorf("response = %+v, want AppName=web, Mode and Allow both empty (unconfigured, open egress)", got)
	}
}

func TestHandleGetAppEgressPolicy_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/egress-policy", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetAppEgressPolicy_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	body := `{"mode":"allowlist","allow":[{"host":"api.anthropic.com","port":443},{"host":"github.com","port":443}]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/egress-policy", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got egressPolicyResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := egressPolicyResource{
		AppName: "web", Mode: "allowlist",
		Allow: []egressAllowResource{{Host: "api.anthropic.com", Port: 443}, {Host: "github.com", Port: 443}},
	}
	if got.AppName != want.AppName || got.Mode != want.Mode || len(got.Allow) != len(want.Allow) {
		t.Fatalf("response = %+v, want %+v", got, want)
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.Egress == nil || saved.Egress.Mode != store.EgressModeAllowlist || len(saved.Egress.Allow) != 2 {
		t.Errorf("saved Egress = %+v", saved.Egress)
	}
}

func TestHandleSetAppEgressPolicy_InvalidMode(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/egress-policy", `{"mode":"deny-all","allow":[{"host":"api.example.com","port":443}]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppEgressPolicy_EmptyAllow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/egress-policy", `{"mode":"allowlist","allow":[]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppEgressPolicy_InvalidPort(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/egress-policy", `{"mode":"allowlist","allow":[{"host":"api.example.com","port":70000}]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppEgressPolicy_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/missing/egress-policy", `{"mode":"allowlist","allow":[{"host":"api.example.com","port":443}]}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleClearAppEgressPolicy_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.UpdateServiceEgressPolicy(context.Background(), "web", &store.ServiceEgressPolicy{
		Mode: store.EgressModeAllowlist, Allow: []store.ServiceEgressAllow{{Host: "api.example.com", Port: 443}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/egress-policy", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.Egress != nil {
		t.Errorf("Egress = %+v, want nil after clearing", saved.Egress)
	}
}

func TestHandleClearAppEgressPolicy_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/missing/egress-policy", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
