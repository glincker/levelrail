package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/store"
)

type provisionDBStore struct {
	DatabaseStore
	events []store.DatabaseProvisionEvent
}

func (p *provisionDBStore) AppendDatabaseProvisionEvent(_ context.Context, e store.DatabaseProvisionEvent) error {
	p.events = append(p.events, e)
	return nil
}

func (p *provisionDBStore) BeginDatabaseProvisionAttempt(context.Context, string, int) (string, error) {
	return "prov_1", nil
}

func (p *provisionDBStore) LatestDatabaseProvisionAttempt(context.Context, string) (string, []store.DatabaseProvisionEvent, error) {
	if len(p.events) == 0 {
		return "", nil, nil
	}
	return "prov_1", p.events, nil
}

func getProvision(t *testing.T, rt *Router) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/provision", nil)
	req.SetPathValue("name", "main")
	rec := httptest.NewRecorder()
	rt.handleGetDatabaseProvision(rec, req)
	return rec
}

func TestGetDatabaseProvision(t *testing.T) {
	ps := &provisionDBStore{}
	rt := &Router{logger: slog.Default(), databases: ps}
	if rec := getProvision(t, rt); rec.Code != http.StatusNotFound {
		t.Fatalf("no attempt: status = %d, want 404", rec.Code)
	}

	tracker, err := dbprovision.NewRecorder(ps).Begin(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	tracker.Done(dbprovision.StepDockerCheck, "ok")
	tracker.Start(dbprovision.StepPullImage, "pulling")
	rec := getProvision(t, rt)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var a dbprovision.Attempt
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.Status != dbprovision.AttemptInProgress || a.CurrentStep != dbprovision.StepPullImage || len(a.Steps) != len(dbprovision.Steps) {
		t.Fatalf("attempt = %+v", a)
	}

	tracker.Fail(dbprovision.StepPullImage, context.DeadlineExceeded)
	var failed dbprovision.Attempt
	_ = json.Unmarshal(getProvision(t, rt).Body.Bytes(), &failed)
	if failed.Failure == nil || failed.Failure.Step != dbprovision.StepPullImage || failed.Status != dbprovision.AttemptFailed {
		t.Fatalf("failed attempt = %+v", failed)
	}
}
