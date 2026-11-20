package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleRestartStopApp_StaticSiteConflict(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "blog", Image: "blog:pending", Port: 80}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveStaticSite(ctx, store.StaticSite{Name: "blog", RootDir: "/srv/blog"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	for _, action := range []string{"restart", "stop"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/blog/"+action, ""))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s status = %d, want 409, body = %s", action, rec.Code, rec.Body.String())
		}
	}
}
