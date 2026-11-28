package api

import (
	"context"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestRecordRefusedAttemptStoresReasonAndCode(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	svc := store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	rt.recordRefusedAttempt(ctx, svc, "nginx:2", store.DeployAttemptSourceImage, errCodeNodeCapacity, "this node is out of room (disk space is down to 1.5% free)")

	got, err := db.ListDeployAttempts(ctx, "web")
	if err != nil || len(got) != 1 {
		t.Fatalf("attempts = %v, err = %v", got, err)
	}
	a := got[0]
	if a.Status != store.DeployAttemptStatusRefused || a.Reason != errCodeNodeCapacity || a.FinishedAt == nil {
		t.Fatalf("attempt = %+v", a)
	}
	if !strings.Contains(a.Error, "disk space is down to 1.5% free") {
		t.Fatalf("error = %q", a.Error)
	}
	if o := computeDeployOutcome(a, true, nil); o != OutcomeRefused {
		t.Fatalf("outcome = %q, want refused", o)
	}
}
