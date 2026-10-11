package upgradesched

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEdgeTargetIsNeverScheduledOrApplied(t *testing.T) {
	t.Run("schedule without a target refuses an edge latest", func(t *testing.T) {
		h := newHarness(t)
		h.host.latest = Candidate{Tag: "main-07cd3bf"}
		h.host.latestErr = ErrEdgeTarget
		_, err := h.sched().Schedule(context.Background(), Request{At: h.now.Add(time.Hour), Actor: "root"})
		if !errors.Is(err, ErrEdgeTarget) {
			t.Fatalf("err = %v, want ErrEdgeTarget", err)
		}
		if h.active() != nil {
			t.Fatal("nothing may be stored for an edge target")
		}
	})
	t.Run("schedule with an explicit edge target refuses it", func(t *testing.T) {
		h := newHarness(t)
		h.host.planErr = ErrEdgeTarget
		_, err := h.sched().Schedule(context.Background(), Request{Target: "main-07cd3bf", At: h.now.Add(time.Hour), Actor: "root"})
		if !errors.Is(err, ErrEdgeTarget) {
			t.Fatalf("err = %v, want ErrEdgeTarget", err)
		}
	})
	t.Run("automatic policy holds with a plain reason", func(t *testing.T) {
		h := newHarness(t)
		h.setPolicy(autoPolicy("any"))
		h.host.latest = Candidate{Tag: "main-07cd3bf"}
		h.host.latestErr = ErrEdgeTarget
		h.tick(h.sched())
		if h.active() != nil {
			t.Fatal("automatic upgrades must not schedule an edge image")
		}
		row, _ := h.db.GetUpgradePolicy(context.Background())
		if !strings.Contains(row.Decision, "container images") {
			t.Fatalf("decision = %q", row.Decision)
		}
	})
	t.Run("fire time re-check treats an edge target as not installable", func(t *testing.T) {
		h := newHarness(t)
		s := h.sched()
		runAt := h.now.Add(time.Hour)
		h.schedule(s, runAt)
		h.host.preflight = []PreflightResult{{Edge: true}}
		h.now = runAt
		h.tick(s)
		got := h.latestRow()
		if got.State != store.UpgradeScheduleFailed || len(h.host.applied) != 0 || h.host.backups != 0 {
			t.Fatalf("got %+v applied=%v backups=%d", got, h.host.applied, h.host.backups)
		}
		if !strings.Contains(got.LastReason, "edge container image") {
			t.Fatalf("reason = %q", got.LastReason)
		}
	})
}
