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

type inspectResolver struct {
	stubResolver
	id  string
	err error
}

func (r inspectResolver) InspectImageID(context.Context, string) (string, error) { return r.id, r.err }

func seedTarget(t *testing.T, db *store.DB, a store.DeployAttempt) {
	t.Helper()
	if a.ServiceName == "" {
		a.ServiceName = "web"
	}
	if a.Status == "" {
		a.Status = store.DeployAttemptStatusSucceeded
	}
	a.StartedAt = time.Now().Add(-time.Hour)
	if err := db.SaveDeployAttempt(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func rollbackTo(rt *Router, cookie *http.Cookie, t *testing.T, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys/"+id+"/rollback", body))
}

func TestRollbackTo_RegistryDigestPinsTheRef(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "nginx:1", ImageDigest: "sha256:aaa", DigestReason: store.DigestReasonResolved})

	rec := rollbackTo(rt, cookie, t, "dep_old", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "nginx:1@sha256:aaa" {
		t.Fatalf("image = %q, want the digest-pinned ref", svc.Image)
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if len(attempts) != 2 || !strings.Contains(attempts[0].Reason, "RollbackTo: dep_old") || attempts[0].ImageDigest != "sha256:aaa" {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestRollbackTo_LocalImage(t *testing.T) {
	target := store.DeployAttempt{ID: "dep_old", Image: "web:abc", ImageDigest: "sha256:local", DigestReason: store.DigestReasonLocalBuild}
	tests := []struct {
		name     string
		resolver inspectResolver
		want     int
		wantMsg  string
	}{
		{"still present", inspectResolver{id: "sha256:local"}, http.StatusAccepted, ""},
		{"garbage collected", inspectResolver{id: ""}, http.StatusGone, "garbage collected"},
		{"tag moved to other content", inspectResolver{id: "sha256:other"}, http.StatusConflict, "different content"},
		{"inspect fails", inspectResolver{err: context.DeadlineExceeded}, http.StatusBadGateway, "could not inspect"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, cookie := newSafetyRouter(t, tt.resolver)
			seedTarget(t, db, target)
			rec := rollbackTo(rt, cookie, t, "dep_old", "")
			if rec.Code != tt.want || !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
			}
			svc, _ := db.GetDesiredService(context.Background(), "web")
			if tt.want != http.StatusAccepted && svc.Image != "nginx:1" {
				t.Fatalf("a refused rollback changed the serving image to %q", svc.Image)
			}
		})
	}
}

func TestRollbackTo_RefusesUnsuitableTargets(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_failed", Image: "nginx:2", Status: store.DeployAttemptStatusFailed, ImageDigest: "sha256:f", DigestReason: store.DigestReasonResolved})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_nodigest", Image: "nginx:3"})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_queued", Image: "nginx:4", Status: store.DeployAttemptStatusQueued})
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "api", Image: "api:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	seedTarget(t, db, store.DeployAttempt{ID: "dep_other", ServiceName: "api", Image: "api:1", ImageDigest: "sha256:x", DigestReason: store.DigestReasonResolved})

	for id, want := range map[string]int{
		"dep_failed": http.StatusConflict, "dep_nodigest": http.StatusConflict, "dep_queued": http.StatusConflict,
		"dep_other": http.StatusNotFound, "dep_missing": http.StatusNotFound,
	} {
		if rec := rollbackTo(rt, cookie, t, id, ""); rec.Code != want {
			t.Errorf("%s: status = %d, want %d (%s)", id, rec.Code, want, rec.Body.String())
		}
	}
}

func TestRollbackTo_RespectsFreezeWindow(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "nginx:1", ImageDigest: "sha256:aaa", DigestReason: store.DigestReasonResolved})
	if put := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploy-freeze", alwaysFrozen())); put.Code != http.StatusOK {
		t.Fatalf("put freeze = %d", put.Code)
	}
	if rec := rollbackTo(rt, cookie, t, "dep_old", ""); rec.Code != http.StatusLocked {
		t.Fatalf("frozen rollback = %d %s", rec.Code, rec.Body.String())
	}
	rec := rollbackTo(rt, cookie, t, "dep_old", `{"override_freeze":true,"override_reason":"hotfix"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("override rollback = %d %s", rec.Code, rec.Body.String())
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if !strings.Contains(attempts[0].Reason, "FreezeOverride: hotfix") || !strings.Contains(attempts[0].Reason, "RollbackTo: dep_old") {
		t.Fatalf("reason = %q", attempts[0].Reason)
	}
}

func TestRollbackTo_ProtectedEnvironmentNeedsApproval(t *testing.T) {
	rt, db, requester, _ := newProtectedSafetyRouter(t, inspectResolver{})
	seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "levelrail/web:0", ImageDigest: "sha256:aaa", DigestReason: store.DigestReasonResolved})

	rec := rollbackTo(rt, requester, t, "dep_old", `{"confirm":true}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "pending_approval") {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "levelrail/web:1" {
		t.Fatalf("image changed before approval: %q", svc.Image)
	}
	approvals, _ := rt.deployApprovals.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if len(approvals) != 1 || approvals[0].Image != "levelrail/web:0@sha256:aaa" {
		t.Fatalf("approvals = %+v", approvals)
	}
}
