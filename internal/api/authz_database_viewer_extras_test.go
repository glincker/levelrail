package api

import (
	"net/http"
	"testing"

	"github.com/glincker/levelrail/internal/store"
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

	// Branch routes check the path database first, so an explicit Deny on it answers 403 even for a branch that does not exist.
	rows := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/databases/victim/engine-stats", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/advisor", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/explorer/capabilities", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/branches/b1/schema-diff", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/redis/info", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/redis/slowlog", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/redis/clients", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/query-history", http.StatusForbidden},
		{http.MethodDelete, "/api/v1/databases/victim/query-history", http.StatusForbidden},
		{http.MethodGet, "/api/v1/databases/victim/saved-queries", http.StatusForbidden},
		{http.MethodPost, "/api/v1/databases/victim/saved-queries", http.StatusForbidden},
		{http.MethodDelete, "/api/v1/databases/victim/saved-queries/q1", http.StatusForbidden},
	}
	for _, row := range rows {
		t.Run(row.method+" "+row.path, func(t *testing.T) {
			if got := matrixDo(rt, row.method, row.path, func(req *http.Request) { req.AddCookie(cookie) }); got != row.want {
				t.Errorf("session status = %d, want %d", got, row.want)
			}
			if got := matrixDo(rt, row.method, row.path, bearer(tok)); got != row.want {
				t.Errorf("token status = %d, want %d", got, row.want)
			}
		})
	}
}
