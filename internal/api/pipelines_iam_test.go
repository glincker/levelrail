package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPipelineAppReadRoutes_IAMDenyScopedToApp(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	rt.SetPipelineSync(&fakeSyncer{}, db)
	bootstrapTestAdmin(t, db)
	seedScheduledTaskApp(t, db, "secret-app")
	seedScheduledTaskApp(t, db, "open-app")
	reader := storeUserWithAbilitiesForTest(t, db, "pipe-reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	attachTestPolicy(t, db, "deny-secret-app-read", "Deny", AbilityRead, "app:secret-app", store.PrincipalTypeUser, reader.ID)

	suffixes := []string{"pipelines", "pipeline-runs", "pipeline-triggers", "pipeline-sync"}
	for _, suffix := range suffixes {
		t.Run(suffix, func(t *testing.T) {
			denied := httptest.NewRecorder()
			rt.Handler().ServeHTTP(denied, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/secret-app/"+suffix, ""))
			if denied.Code != http.StatusForbidden {
				t.Errorf("secret-app status = %d, want %d, body = %s", denied.Code, http.StatusForbidden, denied.Body.String())
			}
			allowed := httptest.NewRecorder()
			rt.Handler().ServeHTTP(allowed, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/open-app/"+suffix, ""))
			if allowed.Code != http.StatusOK {
				t.Errorf("open-app status = %d, want %d, body = %s", allowed.Code, http.StatusOK, allowed.Body.String())
			}
		})
	}
}
