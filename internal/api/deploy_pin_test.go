package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestDeployPin_CrossTeamDenied(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	ctx := context.Background()
	a := store.DeployAttempt{ID: "att_b", ServiceName: appB, Image: "nginx:1", Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}
	if err := f.db.SaveDeployAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleOwner], method, "/api/v1/apps/"+appB+"/deploys/att_b/pin", ""))
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("%s on another team's deploy = %d, want 403 or 404; body = %s", method, rec.Code, rec.Body.String())
		}
	}
	pinned, err := f.db.ListPinnedDeploys(ctx, appB)
	if err != nil || len(pinned) != 0 {
		t.Errorf("a denied pin must not be stored: %v, %v", pinned, err)
	}
}

func TestDeployPin_PinAndUnpinShowInList(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveAttempt(t, db, "att_ok", store.DeployAttemptStatusSucceeded)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploys/att_ok/pin", ""))
	if rec.Code != http.StatusOK || !jsonHasPinned(rec.Body.String(), true) {
		t.Fatalf("pin = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/deploys/att_ok/pin", ""))
	if rec.Code != http.StatusOK || !jsonHasPinned(rec.Body.String(), false) {
		t.Fatalf("unpin = %d %s", rec.Code, rec.Body.String())
	}
}

func jsonHasPinned(body string, want bool) bool {
	return strings.Contains(body, fmt.Sprintf(`"pinned":%t`, want))
}
