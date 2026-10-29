package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

const releaseDigest1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

// seedSourceRelease gives web-staging a succeeded digest attempt, which the
// installed shadow writer turns into a release bound to the source.
func seedSourceRelease(t *testing.T, db *store.DB) *store.Release {
	t.Helper()
	ctx := context.Background()
	release.Install(db, release.ModeOn, nil)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "src1", ServiceName: "web-staging", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeployAttemptDigest(ctx, "src1", releaseDigest1, store.DigestReasonResolved); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "src1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	rel, err := db.GetReleaseByAttempt(ctx, "src1")
	if err != nil || rel == nil {
		t.Fatalf("source release = %v, %v", rel, err)
	}
	return rel
}

func TestPromoteBindingsOffLeavesStoreAlone(t *testing.T) {
	t.Setenv(release.EnvBindings, "off")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	target, _ := db.GetDesiredService(context.Background(), "web-prod")
	if target.Image != "levelrail/web:2" {
		t.Errorf("image = %q, want the tag", target.Image)
	}
	if b, _ := db.GetEnvironmentBinding(context.Background(), "web-prod"); b != nil {
		t.Errorf("binding written while off: %+v", b)
	}
}

func TestPromoteBindingsOnRetargetsBinding(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	rel := seedSourceRelease(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	target, _ := db.GetDesiredService(ctx, "web-prod")
	if target.Image != rel.ImageRef || !strings.Contains(target.Image, "@"+releaseDigest1) {
		t.Errorf("image = %q, want the release digest ref %q", target.Image, rel.ImageRef)
	}
	b, _ := db.GetEnvironmentBinding(ctx, "web-prod")
	if b == nil || b.ReleaseID != rel.ID || b.EnvironmentID != "env_prod" {
		t.Fatalf("binding = %+v, want release %s", b, rel.ID)
	}
	hist, _ := db.ListBindingHistory(ctx, "web-prod")
	if len(hist) != 1 || hist[0].Reason != "promote" {
		t.Errorf("history = %+v, want one promote row", hist)
	}
}

func TestPromoteBindingsOnProtectedStoresReleaseAndApplyIgnoresMovedTag(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := context.Background()
	if err := db.SetEnvironmentProtected(ctx, "env_prod", true); err != nil {
		t.Fatal(err)
	}
	rel := seedSourceRelease(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true}`))
	var result deployTriggerResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.PendingApproval == nil {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	approval, err := db.GetDeployApproval(ctx, result.PendingApproval.ID)
	if err != nil || approval.ReleaseID != rel.ID {
		t.Fatalf("approval release = %q (%v), want %s", approval.ReleaseID, err, rel.ID)
	}

	src, _ := db.GetDesiredService(ctx, "web-staging")
	src.Image = "levelrail/web:moved"
	if err := db.SaveDesiredService(ctx, *src); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.applyClaimedApproval(ctx, approval); err != nil {
		t.Fatalf("apply: %v", err)
	}
	target, _ := db.GetDesiredService(ctx, "web-prod")
	if target.Image != rel.ImageRef {
		t.Errorf("image = %q, want release ref %q despite the moved tag", target.Image, rel.ImageRef)
	}
	hist, _ := db.ListBindingHistory(ctx, "web-prod")
	if len(hist) != 1 || hist[0].ApprovalID != approval.ID {
		t.Errorf("history = %+v, want the approval id recorded", hist)
	}
}

func TestRollbackBindingsOnRetargetsToPreviousRelease(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	ctx := context.Background()
	seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "nginx:1", ImageDigest: "sha256:aaa", DigestReason: store.DigestReasonResolved})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_new", Image: "nginx:2", ImageDigest: "sha256:bbb", DigestReason: store.DigestReasonResolved})
	for _, r := range []store.Release{
		{ID: "rel_old", SourceApp: "web", SourceDeployAttemptID: "dep_old", ImageRef: "nginx@sha256:aaa", Config: "{}"},
		{ID: "rel_new", SourceApp: "web", SourceDeployAttemptID: "dep_new", ImageRef: "nginx@sha256:bbb", Config: "{}"},
	} {
		if _, err := db.InsertRelease(ctx, r); err != nil {
			t.Fatal(err)
		}
		if _, err := db.RecordShadowBinding(ctx, "web", "", r.ID, "image", "tester"); err != nil {
			t.Fatal(err)
		}
	}

	rec := rollbackTo(rt, cookie, t, "dep_old", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	b, _ := db.GetEnvironmentBinding(ctx, "web")
	if b == nil || b.ReleaseID != "rel_old" || b.PreviousReleaseID != "rel_new" {
		t.Fatalf("binding = %+v, want rel_old after rel_new", b)
	}
	hist, _ := db.ListBindingHistory(ctx, "web")
	if last := hist[len(hist)-1]; last.Reason != "rollback" || last.ReleaseID != "rel_old" {
		t.Errorf("last history = %+v", last)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Image != "nginx:1@sha256:aaa" {
		t.Errorf("image = %q, want the digest ref", svc.Image)
	}
}
