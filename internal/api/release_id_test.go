package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedOtherAppRelease(t *testing.T, db *store.DB) *store.Release {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "other-app", Image: "levelrail/other:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "oth1", ServiceName: "other-app", Image: "levelrail/other:1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeployAttemptDigest(ctx, "oth1", "sha256:3333333333333333333333333333333333333333333333333333333333333333", store.DigestReasonResolved); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "oth1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	rel, err := db.GetReleaseByAttempt(ctx, "oth1")
	if err != nil || rel == nil {
		t.Fatalf("other release = %v, %v", rel, err)
	}
	return rel
}

func TestReleaseIDOfAnotherAppIsRejected(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	own := seedSourceRelease(t, db)
	foreign := seedOtherAppRelease(t, db)

	cases := []struct{ path, body string }{
		{"/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true,"release_id":"` + foreign.ID + `"}`},
		{"/api/v1/apps/web-staging/deploys", `{"release_id":"` + foreign.ID + `"}`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, c.path, c.body))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with foreign release = %d %s, want 404", c.path, rec.Code, rec.Body.String())
		}
	}
	if b, _ := db.GetEnvironmentBinding(context.Background(), "web-prod"); b != nil {
		t.Errorf("binding moved by a rejected request: %+v", b)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true,"release_id":"`+own.ID+`"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("own release promote = %d %s", rec.Code, rec.Body.String())
	}
	if target, _ := db.GetDesiredService(context.Background(), "web-prod"); !strings.Contains(target.Image, "@sha256:1111") {
		t.Errorf("image = %q, want the own release pinned ref", target.Image)
	}
}
