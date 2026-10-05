package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type orphanTestRuntime struct {
	docker.Runtime
	containers []docker.ContainerState
}

func (r *orphanTestRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return r.containers, nil
}

func TestOrphansEndpoints(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/orphans", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured status = %d, want 501", rec.Code)
	}

	svc := store.DesiredService{Name: "web", Image: "i:1", Port: 80}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	label := map[string]string{spec.InstanceLabelKey: "inst"}
	live := application.ContainerName("web", application.NameImage(svc), "")
	fake := &orphanTestRuntime{containers: []docker.ContainerState{
		{ID: "1", Name: live, Labels: label},
		{ID: "2", Name: live + "-egress", Labels: label},
		{ID: "3", Name: application.ContainerName("gone", "i:1", ""), Labels: label},
	}}
	rt.SetOrphanReaper(orphans.New(orphans.Deps{
		Store:      db,
		NodeIDs:    func(context.Context) ([]string, error) { return []string{""}, nil },
		Resolve:    func(string) (docker.Runtime, error) { return fake, nil },
		InstanceID: "inst",
		Config:     orphans.DefaultConfig(),
		Logger:     slog.New(slog.DiscardHandler),
	}))

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/orphans", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rep orphans.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Reason != orphans.ReasonServiceDeleted {
		t.Fatalf("findings = %+v, want only the deleted app's container (the live app and its egress sidecar are managed)", rep.Findings)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/orphans/reap?dry_run=true", ""))
	if rec.Code != http.StatusOK || len(fake.containers) != 3 {
		t.Fatalf("dry run: status = %d, containers = %d, want 200 and nothing removed", rec.Code, len(fake.containers))
	}
}
