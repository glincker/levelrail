package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func holdStaticBuild(t *testing.T, db *store.DB, id, dir string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: "web", Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
		ImageDigest: "sha256:abc", DigestReason: store.DigestReasonStaticTree,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveHeldStaticBuild(ctx, id, store.StaticSite{Name: "web", RootDir: dir}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestPublishLock_PublishHeldStaticBuild(t *testing.T) {
	tests := []struct {
		name     string
		removed  bool
		wantCode int
		wantLive bool
	}{
		{name: "held static build goes live", wantCode: http.StatusOK, wantLive: true},
		{name: "missing tree is refused and stays held", removed: true, wantCode: http.StatusGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, cookie := newLockRouter(t)
			dir := filepath.Join(t.TempDir(), "abc")
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			holdStaticBuild(t, db.DB, "dep_static", dir)
			if tt.removed {
				_ = os.RemoveAll(dir)
			}
			rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
			var list []deployAttemptResource
			_ = json.Unmarshal(rec.Body.Bytes(), &list)
			if len(list) != 1 || list[0].PublishState != publishStateHeld || list[0].Live {
				t.Fatalf("held static attempt = %+v, want held and not live", list)
			}
			if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_static/publish", ""); got != tt.wantCode {
				t.Fatalf("publish = %d, want %d", got, tt.wantCode)
			}
			site, err := db.GetStaticSite(context.Background(), "web")
			if tt.wantLive && (err != nil || site.RootDir != dir) {
				t.Fatalf("live site = %+v, %v, want root %q", site, err, dir)
			}
			if !tt.wantLive && err == nil {
				t.Fatalf("site went live despite %d", tt.wantCode)
			}
			ids, _ := db.ListHeldBuildIDs(context.Background(), "web")
			if tt.wantLive == ids["dep_static"] {
				t.Fatalf("held ids = %v after publish", ids)
			}
		})
	}
}

func TestPublishLock_CrashMidStaticPublishConvergesOnRetry(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	dir := t.TempDir()
	holdStaticBuild(t, db.DB, "dep_static", dir)
	db.failMark = true
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_static/publish", ""); got != http.StatusOK {
		t.Fatalf("publish = %d", got)
	}
	if site, err := db.GetStaticSite(context.Background(), "web"); err != nil || site.RootDir != dir {
		t.Fatalf("site must be written before the held row is retired: %+v, %v", site, err)
	}
	db.failMark = false
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_static/publish", ""); got != http.StatusOK {
		t.Fatalf("retry = %d", got)
	}
	if ids, _ := db.ListHeldBuildIDs(context.Background(), "web"); len(ids) != 0 {
		t.Fatalf("held after retry = %v", ids)
	}
}

func TestPublishLock_UnlockKeepsHeldStaticBuildPublishable(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	dir := t.TempDir()
	holdStaticBuild(t, db.DB, "dep_static", dir)
	if got := lockCall(t, rt, cookie, http.MethodDelete, "/publish-lock", ""); got != http.StatusOK {
		t.Fatalf("unlock = %d", got)
	}
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_static/publish", ""); got != http.StatusOK {
		t.Fatalf("publish after unlock = %d", got)
	}
}

func TestPublishLock_LockedImageTriggerHeldThenPublished(t *testing.T) {
	rt, db, cookie := newLockRouter(t)
	if got := lockCall(t, rt, cookie, http.MethodPut, "/publish-lock", `{"reason":"freeze"}`); got != http.StatusOK {
		t.Fatalf("lock = %d", got)
	}
	live, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	trigger := deploy.ImageTrigger{Store: db.DB, Source: store.DeployAttemptSourceImage, Automatic: true, Locks: rt.deployPublishLocks()}
	if _, err := trigger.Deploy(context.Background(), *live, "web:auto"); err != nil {
		t.Fatal(err)
	}
	if liveImage(t, db.DB) != "web:live" {
		t.Fatal("locked image trigger went live")
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	var list []deployAttemptResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].PublishState != publishStateHeld {
		t.Fatalf("attempts = %+v, want one held", list)
	}
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/"+list[0].ID+"/publish", ""); got != http.StatusOK {
		t.Fatalf("publish = %d", got)
	}
	if liveImage(t, db.DB) != "web:auto" {
		t.Fatalf("live image = %q after publish", liveImage(t, db.DB))
	}
}
