package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

// deployOlderRelease binds web-staging to a newer release, then deploys the older one by id.
func deployOlderRelease(t *testing.T, rollback bool) string {
	t.Helper()
	t.Setenv(release.EnvBindings, "on")
	rt, db, cookie := newGateRouter(t)
	seedPromotionFixture(t, db)
	older := seedSourceRelease(t, db)
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "src2", ServiceName: "web-staging", Image: "levelrail/web:3", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeployAttemptDigest(ctx, "src2", "sha256:4444444444444444444444444444444444444444444444444444444444444444", store.DigestReasonResolved); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "src2", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	body := `{"release_id":"` + older.ID + `"`
	if rollback {
		body += `,"rollback":true`
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/deploys", body+"}"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("deploy = %d %s", rec.Code, rec.Body.String())
	}
	hist, err := db.ListBindingHistory(ctx, "web-staging")
	if err != nil || len(hist) == 0 {
		t.Fatalf("history = %v, %v", hist, err)
	}
	attempts, _ := db.ListDeployAttempts(ctx, "web-staging")
	marked := false
	for _, a := range attempts {
		evs, _ := db.ListGateEvents(ctx, a.ID)
		for _, e := range evs {
			if e.Reason == deploysafety.ReasonOperatorRollback {
				marked = true
			}
		}
	}
	if marked != rollback {
		t.Errorf("operator rollback marked = %v, want %v", marked, rollback)
	}
	return hist[len(hist)-1].Reason
}

func TestReleaseDeployReasonAndOperatorRollbackMarking(t *testing.T) {
	if reason := deployOlderRelease(t, true); reason != release.ReasonRollback {
		t.Errorf("rollback reason = %q", reason)
	}
	if reason := deployOlderRelease(t, false); reason != release.ReasonDeploy {
		t.Errorf("forward reason = %q", reason)
	}
}

func TestPromoteReleaseIDAuthzBeforeOwnership(t *testing.T) {
	t.Setenv(release.EnvBindings, "on")
	rt, db := newTestRouter(t)
	seedPromotionFixture(t, db)
	seedSourceRelease(t, db)
	foreign := seedOtherAppRelease(t, db)
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "ro", Name: "ro", TokenHash: hashToken("ro-secret"), Abilities: []string{AbilityRead}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign.ID, "rel_missing"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true,"release_id":"`+id+`"}`, "ro-secret"))
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "release not found") {
			t.Errorf("release %q: %d %s, want 403 independent of release validity", id, rec.Code, rec.Body.String())
		}
	}
}
