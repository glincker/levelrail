package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A per-database Deny must stop the viewer extras for session and token principals alike.
func TestAuthzDatabaseViewerExtrasDenyRows(t *testing.T) {
	t.Parallel()
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	user := storeUserWithAbilitiesForTest(t, db, "viewer-extras@example.com", []string{AbilityRoot})
	cookie := sessionCookieForTest(t, rt, user.ID)
	tok := seedMatrixToken(t, db, "viewer-extras-tok", []string{AbilityRoot})
	attachTestPolicy(t, db, "viewer-extras-u", "Deny", "*", "database:victim", store.PrincipalTypeUser, user.ID)
	attachTestPolicy(t, db, "viewer-extras-t", "Deny", "*", "database:victim", store.PrincipalTypeToken, "viewer-extras-tok")

	rows := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/databases/victim/engine-stats"},
		{http.MethodGet, "/api/v1/databases/victim/query-history"},
		{http.MethodDelete, "/api/v1/databases/victim/query-history"},
		{http.MethodGet, "/api/v1/databases/victim/saved-queries"},
		{http.MethodPost, "/api/v1/databases/victim/saved-queries"},
		{http.MethodDelete, "/api/v1/databases/victim/saved-queries/q1"},
	}
	for _, row := range rows {
		t.Run(row.method+" "+row.path, func(t *testing.T) {
			if got := matrixDo(rt, row.method, row.path, func(req *http.Request) { req.AddCookie(cookie) }); got != http.StatusForbidden {
				t.Errorf("session status = %d, want 403", got)
			}
			if got := matrixDo(rt, row.method, row.path, bearer(tok)); got != http.StatusForbidden {
				t.Errorf("token status = %d, want 403", got)
			}
		})
	}
}
