package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRolloutStatusAndDisruptionBudget(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 3, Strategy: "rolling"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRolloutPolicy(ctx, store.RolloutPolicy{ServiceName: "web", MaxSurge: "1", MaxUnavailable: "2", MinAvailable: "2"}); err != nil {
		t.Fatal(err)
	}

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/rollout", ""))
	var got rolloutResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("GET rollout = %d %s", rec.Code, rec.Body.String())
	}
	if got.Resolved.MaxUnavailable != 2 || got.Resolved.MinAvailable != 2 || got.Replicas != 3 {
		t.Fatalf("resolved = %+v", got)
	}

	rec = serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/restart", ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "min_available") {
		t.Fatalf("restart breaching budget = %d %s, want 409", rec.Code, rec.Body.String())
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/stop", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("stop breaching budget = %d, want 409", rec.Code)
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/stop?force=true", ""))
	if rec.Code == http.StatusConflict {
		t.Fatalf("forced stop refused: %s", rec.Body.String())
	}
}
