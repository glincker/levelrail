package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/dbviewer"
	"github.com/glincker/levelrail/internal/store"
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
		{http.MethodGet, "/api/v1/databases/victim/dynamo/tables"},
		{http.MethodPost, "/api/v1/databases/victim/dynamo/query"},
		{http.MethodPost, "/api/v1/databases/victim/dynamo/scan"},
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

func newTestGate() *mongoGate {
	return &mongoGate{busy: map[string]int{}, changed: make(chan struct{})}
}

func TestMongoGateLimitsPerDatabaseAndPrincipal(t *testing.T) {
	t.Parallel()
	g := newTestGate()
	ctx := context.Background()
	r1, ok := g.acquire(ctx, "db:a", "p:1", 1, 0)
	if !ok {
		t.Fatal("first acquire must succeed")
	}
	if _, ok := g.acquire(ctx, "db:a", "p:2", 1, 0); ok {
		t.Error("same database must be full")
	}
	if _, ok := g.acquire(ctx, "db:b", "p:1", 1, 0); ok {
		t.Error("same principal must be full")
	}
	r1()
	if r2, ok := g.acquire(ctx, "db:a", "p:1", 1, 0); !ok {
		t.Error("slot must free after release")
	} else {
		r2()
	}
	if len(g.busy) != 0 {
		t.Errorf("busy map leaked: %v", g.busy)
	}
}

func TestMongoHistoryErrorHidesFilterValues(t *testing.T) {
	t.Parallel()
	err := &dbviewer.QueryError{Message: `E11000 duplicate key { email: "a@b.c" }`}
	if got := mongoHistoryError(err); got != "query rejected by server" || strings.Contains(got, "a@b.c") {
		t.Fatalf("history error = %q", got)
	}
	if got := mongoHistoryError(errors.New("boom")); got != "internal error" {
		t.Fatalf("history error = %q", got)
	}
}

func TestMongoGateWaitsBrieflyForARelease(t *testing.T) {
	t.Parallel()
	g := newTestGate()
	ctx := context.Background()
	r1, _ := g.acquire(ctx, "db:a", "p:1", 1, 0)
	go func() {
		time.Sleep(30 * time.Millisecond)
		r1()
	}()
	r2, ok := g.acquire(ctx, "db:a", "p:2", 1, 2*time.Second)
	if !ok {
		t.Fatal("waiter must get the slot after the release")
	}
	r2()
	r3, _ := g.acquire(ctx, "db:a", "p:1", 1, 0)
	if _, ok := g.acquire(ctx, "db:a", "p:2", 1, 20*time.Millisecond); ok {
		t.Error("wait must give up after the bound")
	}
	r3()
}

func TestMongoGateCancelledWaiterHoldsNothing(t *testing.T) {
	t.Parallel()
	g := newTestGate()
	r1, _ := g.acquire(context.Background(), "db:a", "p:1", 1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() {
		_, ok := g.acquire(ctx, "db:a", "p:2", 1, time.Minute)
		done <- ok
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if <-done {
		t.Fatal("cancelled waiter must not get a slot")
	}
	r1()
	if len(g.busy) != 0 {
		t.Errorf("busy map leaked: %v", g.busy)
	}
}
