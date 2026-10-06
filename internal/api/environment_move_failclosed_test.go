package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type failingEnvironmentLookup struct {
	EnvironmentStore
	failID string
}

func (f failingEnvironmentLookup) GetEnvironment(ctx context.Context, id string) (store.Environment, error) {
	if id == f.failID {
		return store.Environment{}, errors.New("database unavailable")
	}
	return f.EnvironmentStore.GetEnvironment(ctx, id)
}

func TestMoveAppEnvironment_FailsClosedWhenTheSourceCannotBeLoaded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedAppAndDB(t, db)
	if err := db.SetServiceEnvironment(context.Background(), "web", "env_production"); err != nil {
		t.Fatal(err)
	}
	rt.environments = failingEnvironmentLookup{EnvironmentStore: rt.environments, failID: "env_production"}

	rec := envReq(rt, cookie, http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env_dev"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("move with an unreadable protected source = %d (%s), want 500 and no move", rec.Code, rec.Body.String())
	}
	if ref, _ := db.EnvironmentOfApp(context.Background(), "web"); ref == nil || ref.ID != "env_production" {
		t.Errorf("app left its protected environment without confirmation: %+v", ref)
	}
}
