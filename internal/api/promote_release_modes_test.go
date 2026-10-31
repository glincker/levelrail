package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

// seedShadowRelease records web-staging's release with the shadow writer, so
// a release and a binding exist but no decision may read them.
func seedShadowRelease(t *testing.T, db *store.DB) *store.Release {
	t.Helper()
	ctx := context.Background()
	release.Install(db, release.ModeShadow, nil)
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
		t.Fatalf("shadow release = %v, %v", rel, err)
	}
	return rel
}

func promoteStaging(t *testing.T, rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", body))
	return rec
}

func TestPromoteShadowRecordsButNeverDecides(t *testing.T) {
	t.Setenv(release.EnvBindings, "shadow")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	rel := seedShadowRelease(t, db)
	ctx := context.Background()

	rec := promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	target, _ := db.GetDesiredService(ctx, "web-prod")
	if target.Image != "levelrail/web:2" {
		t.Errorf("image = %q, want the tag path, not the release ref %q", target.Image, rel.ImageRef)
	}
	if b, _ := db.GetEnvironmentBinding(ctx, "web-prod"); b != nil {
		t.Errorf("promote moved a binding in shadow: %+v", b)
	}
	if b, _ := db.GetEnvironmentBinding(ctx, "web-staging"); b == nil || b.ReleaseID != rel.ID {
		t.Errorf("shadow binding for the source = %+v, want %s recorded", b, rel.ID)
	}

	rec = promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true,"release_id":"`+rel.ID+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("release_id in shadow = %d %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestPromoteShadowProtectedStoresNoReleaseOnApproval(t *testing.T) {
	t.Setenv(release.EnvBindings, "shadow")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := context.Background()
	if err := db.SetEnvironmentProtected(ctx, "env_prod", true); err != nil {
		t.Fatal(err)
	}
	seedShadowRelease(t, db)

	var result deployTriggerResult
	rec := promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.PendingApproval == nil {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	approval, err := db.GetDeployApproval(ctx, result.PendingApproval.ID)
	if err != nil || approval.ReleaseID != "" {
		t.Fatalf("approval release = %q (%v), want none in shadow", approval.ReleaseID, err)
	}
	if _, err := rt.applyClaimedApproval(ctx, approval); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if b, _ := db.GetEnvironmentBinding(ctx, "web-prod"); b != nil {
		t.Errorf("approve moved a binding in shadow: %+v", b)
	}
}

func TestRollbackShadowLeavesBindingAlone(t *testing.T) {
	t.Setenv(release.EnvBindings, "shadow")
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
	if b, _ := db.GetEnvironmentBinding(ctx, "web"); b == nil || b.ReleaseID != "rel_new" {
		t.Errorf("binding = %+v, want it untouched on rel_new in shadow", b)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Image != "nginx:1@sha256:aaa" {
		t.Errorf("image = %q, want the attempt digest ref (same as off)", svc.Image)
	}
}

// A private registry answers the manifest probe with 401. The docker layer
// reports that as an inconclusive error, so promote must still go through.
func TestPromoteBindingsOnPrivateRegistry401Proceeds(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	rel := seedSourceRelease(t, db)
	rt.imageResolver = digestVerifierStub{err: errors.New("unauthorized: authentication required")}

	rec := promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s, want 202 on an inconclusive 401", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if b, _ := db.GetEnvironmentBinding(ctx, "web-prod"); b == nil || b.ReleaseID != rel.ID {
		t.Errorf("binding = %+v, want %s", b, rel.ID)
	}
}

func TestPromoteBindingsOnMissingDigestLeavesBindingAlone(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	seedSourceRelease(t, db)
	rt.imageResolver = digestVerifierStub{}

	rec := promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true}`)
	if rec.Code != http.StatusGone {
		t.Fatalf("promote = %d %s, want 410", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if b, _ := db.GetEnvironmentBinding(ctx, "web-prod"); b != nil {
		t.Errorf("binding moved on a refused promote: %+v", b)
	}
	if target, _ := db.GetDesiredService(ctx, "web-prod"); target.Image != "levelrail/web:1" {
		t.Errorf("image = %q, want unchanged", target.Image)
	}
}

func TestApproveBindingsOnDigestCheckOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		stub     digestVerifierStub
		wantErr  bool
		wantBind bool
	}{
		{"private registry 401 is inconclusive", digestVerifierStub{err: errors.New("unauthorized: authentication required")}, false, true},
		{"gone digest is refused", digestVerifierStub{}, true, false},
		{"present digest applies", digestVerifierStub{exists: true}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(release.EnvBindings, "on")
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedPromotionFixture(t, db)
			ctx := context.Background()
			if err := db.SetEnvironmentProtected(ctx, "env_prod", true); err != nil {
				t.Fatal(err)
			}
			rel := seedSourceRelease(t, db)
			var result deployTriggerResult
			rec := promoteStaging(t, rt, cookie, `{"to":"env_prod","confirm":true}`)
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.PendingApproval == nil {
				t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
			}
			approval, _ := db.GetDeployApproval(ctx, result.PendingApproval.ID)
			rt.imageResolver = c.stub

			_, err := rt.applyClaimedApproval(ctx, approval)
			if (err != nil) != c.wantErr {
				t.Fatalf("apply err = %v, wantErr %v", err, c.wantErr)
			}
			b, _ := db.GetEnvironmentBinding(ctx, "web-prod")
			if c.wantBind && (b == nil || b.ReleaseID != rel.ID) {
				t.Errorf("binding = %+v, want %s", b, rel.ID)
			}
			if !c.wantBind && b != nil {
				t.Errorf("binding = %+v, want none", b)
			}
			if c.wantErr && !strings.Contains(err.Error(), "no longer in its registry") {
				t.Errorf("err = %v, want registry advice", err)
			}
		})
	}
}
