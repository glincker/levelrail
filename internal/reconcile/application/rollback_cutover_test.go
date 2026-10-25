package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func saveFinishedAttempt(t *testing.T, db *store.DB, id, image string, started time.Time) {
	t.Helper()
	ctx := context.Background()
	svc := store.DesiredService{Name: "web", Image: image, Port: 80}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: image, Status: store.DeployAttemptStatusRunning, StartedAt: started, Snapshot: store.NewDeployAttemptSnapshot(svc)}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, started, ""); err != nil {
		t.Fatal(err)
	}
}

// The post-deploy watch arms from the cutover gate event, so a rollback that
// lands on an already running container must record one for its own attempt.
func TestRollbackOntoRunningReleaseRecordsCutover(t *testing.T) {
	tests := []struct {
		name       string
		rollbackTo string
		heldV1     bool
	}{
		{"swap to the held previous release", "img:v1", true},
		{"rollback to the release already live", "img:v2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			srv := alwaysHealthy()
			defer srv.Close()
			rt := newFakeRuntime(serverPort(t, srv))
			desired := gateDesired()
			desired.Strategy = strategyBlueGreen
			rt.setExitState(ContainerName("web", "img:v2", ""), &docker.ExitState{Running: true})
			now := time.Now()
			saveFinishedAttempt(t, db, "dep_v1", "img:v1", now.Add(-time.Hour))
			saveFinishedAttempt(t, db, "dep_v2", "img:v2", now.Add(-time.Minute))
			cfg := deploysafety.Effective{SuccessThreshold: 1, WatchWindow: time.Hour}
			opts := []Option{WithStabilityGate(cfg, nil), WithGateRecorder(db), WithGateTick(10 * time.Millisecond), WithReadyBudget(2 * time.Second), WithPreviousReleaseHold(time.Hour)}
			c := New("web", &fakeStore{svc: desired}, rt, opts...)
			if _, err := c.Reconcile(ctx); err != nil {
				t.Fatalf("deploy Reconcile() error = %v", err)
			}
			if tt.heldV1 {
				rt.seed(ContainerName("web", "img:v1", ""), true)
				rt.mu.Lock()
				rt.containers[ContainerName("web", "img:v1", "")].Created = now.Add(-time.Minute)
				rt.containers[ContainerName("web", "img:v2", "")].Created = now
				rt.mu.Unlock()
			}

			saveFinishedAttempt(t, db, "dep_rb", tt.rollbackTo, time.Now())
			desired.Image = tt.rollbackTo
			for range 2 {
				if _, err := c.Reconcile(ctx); err != nil {
					t.Fatalf("rollback Reconcile() error = %v", err)
				}
			}
			events, err := db.ListGateEvents(ctx, "dep_rb")
			if err != nil {
				t.Fatal(err)
			}
			cutovers := 0
			for _, e := range events {
				if e.Phase == deploysafety.PhaseCutover && e.Status == deploysafety.StatusPassed {
					cutovers++
				}
			}
			if cutovers != 1 {
				t.Errorf("cutover events on the rollback attempt = %d, want 1: %+v", cutovers, events)
			}
		})
	}
}
