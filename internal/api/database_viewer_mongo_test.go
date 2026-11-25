package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A per-database Deny must stop the Mongo viewer routes for session and token principals alike.
func TestAuthzMongoViewerDenyRows(t *testing.T) {
	t.Parallel()
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	user := storeUserWithAbilitiesForTest(t, db, "mongo-viewer@example.com", []string{AbilityRoot})
	cookie := sessionCookieForTest(t, rt, user.ID)
	tok := seedMatrixToken(t, db, "mongo-viewer-tok", []string{AbilityRoot})
	attachTestPolicy(t, db, "mongo-viewer-u", "Deny", "*", "database:victim", store.PrincipalTypeUser, user.ID)
	attachTestPolicy(t, db, "mongo-viewer-t", "Deny", "*", "database:victim", store.PrincipalTypeToken, "mongo-viewer-tok")

	rows := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/databases/victim/mongo/collections"},
		{http.MethodPost, "/api/v1/databases/victim/mongo/find"},
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

func TestMongoGateLimitsPerDatabaseAndPrincipal(t *testing.T) {
	t.Parallel()
	g := &mongoGate{busy: map[string]int{}}
	r1, ok := g.acquire("db:a", "p:1", 1)
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if _, ok := g.acquire("db:a", "p:2", 1); ok {
		t.Error("same database must be full")
	}
	if _, ok := g.acquire("db:b", "p:1", 1); ok {
		t.Error("same principal must be full")
	}
	r1()
	if r2, ok := g.acquire("db:a", "p:1", 1); !ok {
		t.Error("slot must free after release")
	} else {
		r2()
	}
	if len(g.busy) != 0 {
		t.Errorf("busy map leaked: %v", g.busy)
	}
}
