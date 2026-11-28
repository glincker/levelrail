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

func TestAttentionFeed_DeployItemsRespectAppVisibility(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	for _, app := range []string{appA, appB} {
		id, err := store.NewDeployAttemptID()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: app, Image: "nginx:2",
			Status: store.DeployAttemptStatusRefused, Error: "no space", StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookies[store.TeamRoleOwner], http.MethodGet, "/api/v1/attention/feed", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("feed = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, appA) {
		t.Fatalf("team A's own refused deploy missing: %s", body)
	}
	if strings.Contains(body, appB) {
		t.Fatalf("team B's app leaked into team A's feed: %s", body)
	}
}
