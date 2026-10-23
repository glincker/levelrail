package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// flakyLocks fails MarkHeldBuildPublished until failMark is cleared, to
// simulate a crash between writing desired state and retiring the held row.
type flakyLocks struct {
	*store.DB
	failMark bool
}

func (f *flakyLocks) MarkHeldBuildPublished(ctx context.Context, id, by string, at time.Time) error {
	if f.failMark {
		return errors.New("crashed")
	}
	return f.DB.MarkHeldBuildPublished(ctx, id, by, at)
}

func newLockRouter(t *testing.T) (*Router, *flakyLocks, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	locks := &flakyLocks{DB: db}
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithDeploySafety(db, nil), WithPublishLocks(locks))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "web:live", Port: 80}); err != nil {
		t.Fatal(err)
	}
	return rt, locks, cookie
}

func lockCall(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) int {
	t.Helper()
	return serve(rt, authedRequest(t, cookie, method, "/api/v1/apps/web"+path, body)).Code
}

func holdBuild(t *testing.T, db *store.DB, id, image string) {
	t.Helper()
	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{ID: id, ServiceName: "web", Image: image, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveHeldBuild(context.Background(), id, store.DesiredService{Name: "web", Image: image, Port: 80}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func liveImage(t *testing.T, db *store.DB) string {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	return svc.Image
}

func TestPublishLock_LockUnlockKeepsFirstLock(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	if got := lockCall(t, rt, cookie, http.MethodPut, "/publish-lock", `{"reason":"release freeze"}`); got != http.StatusOK {
		t.Fatalf("lock = %d", got)
	}
	if got := lockCall(t, rt, cookie, http.MethodPut, "/publish-lock", `{"reason":"second"}`); got != http.StatusOK {
		t.Fatalf("relock = %d", got)
	}
	lock, _ := db.GetPublishLock(context.Background(), "web")
	if lock == nil || lock.Reason != "release freeze" || lock.PinnedImage != "web:live" || lock.LockedBy == "" {
		t.Fatalf("lock = %+v", lock)
	}
	if got := lockCall(t, rt, cookie, http.MethodDelete, "/publish-lock", ""); got != http.StatusOK {
		t.Fatalf("unlock = %d", got)
	}
	if lock, _ := db.GetPublishLock(context.Background(), "web"); lock != nil {
		t.Fatalf("lock after unlock = %+v", lock)
	}
}

func TestPublishLock_PublishHeldBuild(t *testing.T) {
	tests := []struct {
		name     string
		deployID string
		want     int
		wantLive string
	}{
		{"held build goes live", "dep_held", http.StatusOK, "web:new"},
		{"unknown id", "dep_missing", http.StatusNotFound, "web:live"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, cookie := newLockRouter(t)
			holdBuild(t, db.DB, "dep_held", "web:new")
			if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/"+tt.deployID+"/publish", ""); got != tt.want {
				t.Fatalf("publish = %d, want %d", got, tt.want)
			}
			if got := liveImage(t, db.DB); got != tt.wantLive {
				t.Errorf("live image = %q, want %q", got, tt.wantLive)
			}
		})
	}
}

func TestPublishLock_HeldBuildsSurviveUnlockAndShowInHistory(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	holdBuild(t, db.DB, "dep_held", "web:new")
	lockCall(t, rt, cookie, http.MethodDelete, "/publish-lock", "")

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	var list []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %v %s", err, rec.Body.String())
	}
	if list[0].PublishState != publishStateHeld || list[0].Live {
		t.Fatalf("attempt = %+v, want held and not live", list[0])
	}
}

func TestPublishLock_CrashMidPublishConvergesOnRetry(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	holdBuild(t, db.DB, "dep_held", "web:new")
	db.failMark = true
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_held/publish", ""); got != http.StatusOK {
		t.Fatalf("publish = %d", got)
	}
	if liveImage(t, db.DB) != "web:new" {
		t.Fatal("desired state must be written before the held row is retired")
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	var list []deployAttemptResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].PublishState != "" || !list[0].Live {
		t.Fatalf("half-published attempt = %+v, want live and no longer held", list)
	}
	db.failMark = false
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_held/publish", ""); got != http.StatusOK {
		t.Fatalf("retry = %d", got)
	}
	if ids, _ := db.ListHeldBuildIDs(context.Background(), "web"); len(ids) != 0 {
		t.Fatalf("held after retry = %v", ids)
	}
}

func TestPublishLock_RollbackAndLockLocksEvenWhenRollbackRefused(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	holdBuild(t, db.DB, "dep_old", "")
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_old/rollback-lock", ""); got == http.StatusOK || got == http.StatusAccepted {
		t.Fatalf("rollback of an imageless deploy = %d, want a refusal", got)
	}
	if lock, _ := db.GetPublishLock(context.Background(), "web"); lock == nil {
		t.Fatal("app must stay locked when the rollback half fails")
	}
}
