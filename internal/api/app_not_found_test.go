package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppNotFound_StackNameHintsServices(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"shop-web", "shop-db"} {
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: n, Image: "img:v1", Port: 8080}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for _, c := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/v1/apps/shop", "shop-db, shop-web"},
		{http.MethodPost, "/api/v1/apps/shop/stop", "shop-db, shop-web"},
		{http.MethodGet, "/api/v1/apps/nope", "compose stacks"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, ""))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s %s = %d %s, want 404 containing %q", c.method, c.path, rec.Code, rec.Body.String(), c.want)
		}
	}
}
