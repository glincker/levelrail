package api

import (
	"context"
	"log/slog"
	"slices"

	"github.com/glincker/levelrail/internal/store"
)

// replicaNodesWriter is the store write behind ApplyReplicaSpread, kept off
// AppStore so existing fakes need not grow a method.
type replicaNodesWriter interface {
	UpdateServiceReplicaNodes(ctx context.Context, name string, nodes []string) error
}

// ApplyReplicaSpread plans svc's replica nodes and persists them when they
// changed, clearing a stale spread when the setting is off or the plan refuses.
// It returns svc with ReplicaNodes updated, plus the refusal reason if any.
// Nothing calls it from the reconcile pass yet: ingress already routes by
// ReplicaNodes, so it must stay unwired until the application reconciler
// starts each replica on its node.
func (rt *Router) ApplyReplicaSpread(ctx context.Context, svc store.DesiredService) (store.DesiredService, string) {
	plan := rt.PlanReplicaSpread(ctx, svc)
	if slices.Equal(plan.Nodes, svc.ReplicaNodes) {
		return svc, plan.Refusal
	}
	w, ok := rt.apps.(replicaNodesWriter)
	if !ok {
		return svc, plan.Refusal
	}
	if err := w.UpdateServiceReplicaNodes(ctx, svc.Name, plan.Nodes); err != nil {
		rt.logger.Warn("api: persist replica spread failed", slog.String("service", svc.Name), slog.String("error", err.Error()))
		return svc, plan.Refusal
	}
	svc.ReplicaNodes = plan.Nodes
	return svc, plan.Refusal
}
