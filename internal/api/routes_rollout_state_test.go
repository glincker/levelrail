package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRolloutState_IgnoresRefusedAttempts(t *testing.T) {
	rt, db, cookie := newGateRouter(t)
	id, err := store.NewDeployAttemptID()
	if err != nil {
		t.Fatal(err)
	}
	err = db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: id, ServiceName: "web", Image: "nginx:2", Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusRefused, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/rollout/state", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got rolloutStateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DeployID != "" || got.Image != "" {
		t.Fatalf("refused attempt leaked into rollout state: %+v", got)
	}
}
