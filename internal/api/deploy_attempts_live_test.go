package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListDeployAttempts_OnlyNewestSucceededIsLive(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:2", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	seed := []store.DeployAttempt{
		{ID: "d1", Image: "img:1", StartedAt: base},
		{ID: "d2", Image: "img:2", StartedAt: base.Add(time.Minute)},
		{ID: "d3", Image: "img:1", StartedAt: base.Add(2 * time.Minute)},
		{ID: "d4", Image: "img:2", StartedAt: base.Add(3 * time.Minute)},
	}
	for _, a := range seed {
		a.ServiceName, a.Source, a.Status = "web", store.DeployAttemptSourceImage, store.DeployAttemptStatusSucceeded
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	var got []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	live := map[string]bool{}
	trig := map[string]string{}
	for _, r := range got {
		live[r.ID], trig[r.ID] = r.Live, r.Trigger
	}
	if !live["d4"] || live["d2"] || live["d1"] || live["d3"] {
		t.Errorf("live = %v, want only d4", live)
	}
	if trig["d3"] != store.DeploymentTriggerRollback || trig["d4"] != store.DeploymentTriggerRollback {
		t.Errorf("triggers = %v, want d3 and d4 rollback", trig)
	}
	if trig["d2"] == store.DeploymentTriggerRollback || trig["d1"] == store.DeploymentTriggerRollback {
		t.Errorf("triggers = %v, d1 and d2 are not rollbacks", trig)
	}
}
