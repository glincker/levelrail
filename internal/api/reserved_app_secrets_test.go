package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestReservedApp_SecretRoutesRefuse(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "legacy", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A pre-fix app already carries the reserved name, so rename the row under the guard.
	if _, err := db.ExecContext(ctx, `UPDATE desired_services SET name = 'vault' WHERE name = 'legacy'`); err != nil {
		t.Fatalf("rename: %v", err)
	}
	cases := []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/apps/vault/secrets/credential", `{"value":"x"}`},
		{http.MethodGet, "/api/v1/apps/vault/secrets", ""},
		{http.MethodPut, "/api/v1/apps/vault/secrets/credential/lock", `{"locked":true}`},
		{http.MethodPut, "/api/v1/apps/vault/secrets/credential/build", `{"build_time":true}`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, c.body))
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			continue
		}
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "reserved by the control plane") {
			t.Errorf("%s %s = %d %s, want 409 reserved", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	if setter.calls != 0 {
		t.Fatalf("setter was called %d times for a reserved app", setter.calls)
	}
}

func TestRecentlyDeleted_ReservedDatabaseNameIsNotRestored(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	snap := `{"Name":"vault","Engine":"postgres","Version":"16"}`
	if err := db.AddRecentlyDeleted(context.Background(), store.RecentlyDeleted{Kind: store.RecentlyDeletedDatabase, Name: "vault", Snapshot: snap, DeletedAt: time.Now()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/recently-deleted/database/vault/restore", `{"confirm":"vault"}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "reserved by the control plane") {
		t.Fatalf("restore = %d %s, want 409 reserved", rec.Code, rec.Body.String())
	}
}
