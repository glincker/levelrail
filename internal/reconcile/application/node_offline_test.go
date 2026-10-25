package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func TestNodeOfflineController_ReportsNotReady(t *testing.T) {
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c := NewNodeOfflineController("web", "edge-1", at.Add(-95*time.Second))
	c.now = func() time.Time { return at }
	if c.Name() != ControllerName("web") {
		t.Fatalf("Name() = %q", c.Name())
	}
	res, err := c.Reconcile(context.Background())
	if err != nil || len(res.Conditions) != 1 {
		t.Fatalf("Reconcile() = %+v, %v", res, err)
	}
	cond := res.Conditions[0]
	if cond.Status != reconcile.ConditionFalse || cond.Reason != ReasonNodeOffline {
		t.Fatalf("condition = %+v", cond)
	}
	if !strings.Contains(cond.Message, "edge-1") || !strings.Contains(cond.Message, "1m35s") {
		t.Fatalf("message = %q", cond.Message)
	}
}
