package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const errCodeNodeOffline = "node_offline"

// queueWhileOffline reads the explicit opt-in for lifecycle requests that carry
// no body (restart, start, stop).
func queueWhileOffline(r *http.Request) bool {
	return r.URL.Query().Get("queue_while_offline") == "true"
}

// offlineNodeFor returns the node hosting svc when it is offline, nil when the
// app is local, the node is unknown or the node is reachable.
func (rt *Router) offlineNodeFor(ctx context.Context, svc store.DesiredService) *store.Node {
	if rt.nodes == nil || svc.NodeID == "" {
		return nil
	}
	n, err := rt.nodes.GetNode(ctx, svc.NodeID)
	if err != nil || n == nil || n.Status != store.NodeStatusOffline {
		return nil
	}
	return n
}

func offlineNodeSummary(n *store.Node) string {
	if n.LastSeenAt == nil {
		return fmt.Sprintf("node %s is offline", n.Name)
	}
	return fmt.Sprintf("node %s is offline (unseen for %s)", n.Name, time.Since(*n.LastSeenAt).Round(time.Second))
}

// refuseOfflineNode writes a 409 and reports true when svc's node is offline
// and the caller did not opt in to queueing. Nothing is changed in that case.
func (rt *Router) refuseOfflineNode(ctx context.Context, w http.ResponseWriter, svc store.DesiredService, queue bool) bool {
	n := rt.offlineNodeFor(ctx, svc)
	if n == nil || queue {
		return false
	}
	writeCodedError(w, http.StatusConflict, errCodeNodeOffline,
		offlineNodeSummary(n)+", so this change would not be applied: bring the node back, or move the app with `apps move` / `nodes drain`. "+
			"To record the change anyway and apply it when the node returns, retry with queue_while_offline (CLI --queue-while-offline).")
	return true
}

// recordPendingAttempt stores a change accepted for an offline node as pending,
// never as a succeeded rollout.
func (rt *Router) recordPendingAttempt(ctx context.Context, svc store.DesiredService, image, source string) {
	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: record pending attempt: mint id failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return
	}
	reason := "queued while the node is offline; applies when it reconnects"
	if n := rt.offlineNodeFor(ctx, svc); n != nil {
		reason = "queued while node " + n.Name + " is offline; applies when it reconnects"
	}
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: svc.Name, Image: image, Source: source,
		Status: store.DeployAttemptStatusPending, StartedAt: time.Now(),
		Snapshot: store.NewDeployAttemptSnapshot(svc), Reason: reason,
	}); err != nil {
		rt.logger.Error("api: record pending attempt failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
	}
}
