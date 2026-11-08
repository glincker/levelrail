package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ReasonWaitingForMesh is the Ready condition reason of a service parked
// until its node's WireGuard mesh path is healthy.
const ReasonWaitingForMesh = "WaitingForMesh"

const (
	meshRetryFloor      = 2 * time.Second
	defaultMeshRetryMax = 10 * time.Second
)

// WithMeshRetry sets the longest delay between mesh readiness checks of a
// parked service, read on every pass so a changed setting applies live.
func WithMeshRetry(get func(context.Context) time.Duration) Option {
	return func(c *Controller) { c.meshRetry = get }
}

// meshWaitBlock parks a service on a remote node whose mesh is on but not
// usable yet, instead of starting it into a readiness failure nobody retries.
// A replica already running is never blocked, so a mesh flap cannot flip a
// serving app; a resolver error fails open to the old behavior.
func (c *Controller) meshWaitBlock(ctx context.Context, desired *store.DesiredService) *reconcile.Result {
	if c.meshPaths == nil || desired.NodeID == "" {
		return nil
	}
	path, err := c.meshPaths.Path(ctx, desired.NodeID)
	if err != nil || path.Local || path.NoMesh || path.Usable {
		c.clearMeshWait()
		return nil
	}
	if c.hasRunningReplica(ctx) {
		c.clearMeshWait()
		return nil
	}
	delay, first := c.nextMeshDelay(ctx)
	level := slog.LevelDebug
	if first {
		level = slog.LevelInfo
	}
	slog.Log(ctx, level, "reconcile: service waits for the mesh before starting", slog.String("service", c.serviceName), slog.String("node", desired.NodeID), slog.String("reason", path.Reason))
	return &reconcile.Result{
		Conditions: []reconcile.Condition{{
			Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionUnknown, Reason: ReasonWaitingForMesh,
			Message: fmt.Sprintf("waiting for the WireGuard mesh to node %s before starting: %s. It starts on its own once the mesh is healthy, no redeploy needed", desired.NodeID, path.Reason),
		}},
		RequeueAfter: delay,
		KeyOnly:      true,
	}
}

func (c *Controller) hasRunningReplica(ctx context.Context) bool {
	all, err := c.listPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return false
	}
	for _, cs := range all {
		if cs.Running && ownsContainer(c.serviceName, cs.Name) {
			return true
		}
	}
	return false
}

// nextMeshDelay doubles from the floor up to the configured maximum, so a
// mesh that comes up quickly is noticed fast and a long outage stays cheap.
func (c *Controller) nextMeshDelay(ctx context.Context) (time.Duration, bool) {
	limit := defaultMeshRetryMax
	if c.meshRetry != nil {
		if d := c.meshRetry(ctx); d > 0 {
			limit = d
		}
	}
	c.meshWaitMu.Lock()
	defer c.meshWaitMu.Unlock()
	n := c.meshWaitStreak
	c.meshWaitStreak++
	delay := meshRetryFloor
	for i := 0; i < n && delay < limit; i++ {
		delay *= 2
	}
	return min(delay, limit), n == 0
}

func (c *Controller) clearMeshWait() {
	c.meshWaitMu.Lock()
	c.meshWaitStreak = 0
	c.meshWaitMu.Unlock()
}
