package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRecentlyDeleted_RestoreApp(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000, Env: map[string]string{"A": "b"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	do := func(method, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, `{"confirm":"web"}`))
		return rec
	}
	if rec := do(http.MethodDelete, "/api/v1/apps/web"); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	list := do(http.MethodGet, "/api/v1/recently-deleted")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"web"`) {
		t.Fatalf("list = %d %s, want the deleted app", list.Code, list.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/recently-deleted/app/web/restore"); rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil || got.Image != "levelrail/web:1" || got.Env["A"] != "b" {
		t.Fatalf("restored app = %+v, err = %v", got, err)
	}
	if rec := do(http.MethodPost, "/api/v1/recently-deleted/app/web/restore"); rec.Code != http.StatusNotFound {
		t.Fatalf("second restore status = %d, want 404", rec.Code)
	}
}

func TestRecentlyDeleted_SweepDropsExpired(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	old := store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "old", Snapshot: "{}", DeletedAt: time.Now().Add(-30 * 24 * time.Hour)}
	fresh := store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "fresh", Snapshot: "{}"}
	for _, e := range []store.RecentlyDeleted{old, fresh} {
		if err := db.AddRecentlyDeleted(ctx, e); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if n, err := rt.SweepRecentlyDeleted(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v, want 1 dropped", n, err)
	}
	if _, err := db.GetRecentlyDeleted(ctx, store.RecentlyDeletedApp, "fresh"); err != nil {
		t.Fatalf("fresh entry gone: %v", err)
	}
}
