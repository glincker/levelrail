package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedRetainedCopy(t *testing.T, db *store.DB, app, copySvc string, suspended, sleeping bool) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: copySvc, Image: "img:v0", Port: 3000, Suspended: suspended}); err != nil {
		t.Fatalf("seed copy: %v", err)
	}
	if err := db.UpdateServiceSuspended(ctx, copySvc, suspended); err != nil {
		t.Fatalf("suspend copy: %v", err)
	}
	if err := db.SaveDeployURL(ctx, store.DeployURL{ServiceName: copySvc, AppName: app, AttemptID: "da_" + copySvc, Host: copySvc + ".example.com"}); err != nil {
		t.Fatalf("save deploy url: %v", err)
	}
	if sleeping {
		if err := db.SetAppSleepIdle(ctx, copySvc, 10, time.Now()); err != nil {
			t.Fatalf("sleep setting: %v", err)
		}
		if err := db.SetAppSleeping(ctx, copySvc, true, time.Now()); err != nil {
			t.Fatalf("mark sleeping: %v", err)
		}
	}
}

func TestDeleteAppRemovesRetainedSiblings(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedApp(t, db, "web")
	seedRetainedCopy(t, db, "web", "web-dep1abcd", true, false)
	seedRetainedCopy(t, db, "web", "web-dep2abcd", false, true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, name := range []string{"web", "web-dep1abcd", "web-dep2abcd"} {
		if _, err := db.GetDesiredService(ctx, name); !errors.Is(err, store.ErrServiceNotFound) {
			t.Errorf("%s still present, err = %v", name, err)
		}
	}
	if rows, err := db.ListDeployURLs(ctx, "web"); err != nil || len(rows) != 0 {
		t.Errorf("deploy url rows = %v, err = %v", rows, err)
	}
}

func TestWakeNeverStartsRetainedCopy(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	// Orphan: its app is gone. Stopped: its app lives but the copy gave up after failed readiness.
	seedRetainedCopy(t, db, "gone", "gone-dep1abcd", true, true)
	seedApp(t, db, "live")
	seedRetainedCopy(t, db, "live", "live-dep1abcd", true, false)

	for _, name := range []string{"gone-dep1abcd", "live-dep1abcd"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+name+"/sleep/wake", ""))
		if rec.Code == http.StatusInternalServerError {
			t.Fatalf("%s wake status = %d", name, rec.Code)
		}
		svc, err := db.GetDesiredService(ctx, name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if !svc.Suspended {
			t.Errorf("%s was started by a wake", name)
		}
	}
}

func TestDeleteAppCompletesWhenImageInventoryUnavailable(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithImageInventory(func(string) (docker.ImageInventory, error) {
		return nil, errors.New("docker unavailable")
	}))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedRetainedCopy(t, db, "web", "web-dep1abcd", true, false)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, name := range []string{"web", "web-dep1abcd"} {
		if _, err := db.GetDesiredService(context.Background(), name); !errors.Is(err, store.ErrServiceNotFound) {
			t.Errorf("%s still present, err = %v", name, err)
		}
	}
}

func TestDeleteAppRemovesPushRepoAndRecreatedAppAcceptsNewHistory(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "one"})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("first push: %v", err)
	}
	if e.rt.sourceCache.MirrorSize(gitPushMirrorURL("web")) == 0 {
		t.Fatal("expected a stored push repo")
	}

	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if got := e.rt.sourceCache.MirrorSize(gitPushMirrorURL("web")); got != 0 {
		t.Fatalf("push repo survived the delete: %d bytes", got)
	}

	seedApp(t, e.db, "web")
	token = e.issue(t, "web")
	other, _ := pushWorkRepo(t, map[string]string{"b.txt": "unrelated"})
	if err := pushTo(other, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("push of an unrelated history after re-create: %v", err)
	}
}
