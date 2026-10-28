package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedOneShotHealth(t *testing.T, db *store.DB) {
	t.Helper()
	seedWebAppForTest(t, db)
	h := &store.ServiceHealth{Restart: store.RestartPolicyNo, DependsOnCompleted: []string{"migrate"}}
	if err := db.UpdateServiceHealth(context.Background(), "web", h); err != nil {
		t.Fatal(err)
	}
}

func assertOneShotKept(t *testing.T, h *store.ServiceHealth) {
	t.Helper()
	if h == nil || h.Restart != store.RestartPolicyNo || !slices.Equal(h.DependsOnCompleted, []string{"migrate"}) {
		t.Fatalf("health = %+v, want restart and depends_on_completed kept", h)
	}
}

func TestAppHealthWrite_KeepsRunToCompletion(t *testing.T) {
	tests := []struct {
		name, method, body string
	}{
		{"put readiness", http.MethodPut, `{"readiness":{"path":"/healthz"}}`},
		{"put empty", http.MethodPut, `{}`},
		{"delete", http.MethodDelete, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedOneShotHealth(t, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, "/api/v1/apps/web/health", tt.body))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			saved, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}
			assertOneShotKept(t, saved.Health)
		})
	}
}

func TestPreserveUnsentAppFields_KeepsRunToCompletion(t *testing.T) {
	existing := store.DesiredService{Name: "web", Health: &store.ServiceHealth{Restart: store.RestartPolicyNo, DependsOnCompleted: []string{"migrate"}}}
	for _, sent := range []*store.ServiceHealth{nil, {Readiness: &store.ServiceProbe{Path: "/"}}} {
		desired := store.DesiredService{Name: "web", Health: sent}
		preserveUnsentAppFields(&desired, existing, appResource{Name: "web"}, false)
		assertOneShotKept(t, desired.Health)
	}
}
