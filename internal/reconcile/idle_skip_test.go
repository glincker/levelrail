package reconcile

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type settledCtl struct {
	name string
	n    atomic.Int32
}

func (c *settledCtl) Name() string { return c.name }
func (c *settledCtl) Reconcile(context.Context) (Result, error) {
	c.n.Add(1)
	return Result{Conditions: []Condition{{Type: ConditionTypeReady, Status: ConditionTrue, Reason: "Ready"}}}, nil
}

func TestEventDrivenSkipsQuietAppsOnly(t *testing.T) {
	web, api := &settledCtl{name: "application/web"}, &settledCtl{name: "application/api"}
	e := NewEngine(testLogger())
	e.SetSource(func(context.Context) ([]Controller, error) { return []Controller{web, api}, nil })
	e.SetKeyed(func(n string) bool { return strings.HasPrefix(n, "application/") })
	e.SetEventDriven(true)
	ctx := context.Background()

	e.ReconcileAll(ctx)
	e.ReconcileAll(ctx)
	if web.n.Load() != 1 || api.n.Load() != 1 {
		t.Fatalf("quiet pass ran apps: web=%d api=%d", web.n.Load(), api.n.Load())
	}

	e.markContainerDirty(docker.Event{ContainerName: "web-1a2b3c4d"}.ContainerName)
	e.ReconcileAll(ctx)
	if web.n.Load() != 2 || api.n.Load() != 1 {
		t.Fatalf("event pass: web=%d api=%d, want 2 and 1", web.n.Load(), api.n.Load())
	}

	e.Nudge()
	e.ReconcileAll(ctx)
	if web.n.Load() != 3 || api.n.Load() != 2 {
		t.Fatalf("nudge pass: web=%d api=%d, want 3 and 2", web.n.Load(), api.n.Load())
	}

	e.now = func() time.Time { return time.Now().Add(2 * e.fullResync) }
	e.ReconcileAll(ctx)
	if web.n.Load() != 4 || api.n.Load() != 3 {
		t.Fatalf("full resync pass: web=%d api=%d, want 4 and 3", web.n.Load(), api.n.Load())
	}
}
