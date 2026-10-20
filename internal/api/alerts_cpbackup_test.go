package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCreateAlertRule_ControlPlaneBackupStaleSuccess(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	body := `{"name":"cp backup stale","kind":"control_plane_backup_stale","for_duration":"72h","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}
