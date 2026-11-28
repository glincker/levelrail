package main

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/store"
)

// spreadPlanner picks the node of each replica; nil nodes keep every replica on the home node.
type spreadPlanner func(ctx context.Context, svc *store.DesiredService) ([]string, string)

// routerSpreadPlanner adapts the API router's planner without moving it out of internal/api.
func routerSpreadPlanner(rt *api.Router) spreadPlanner {
	return func(ctx context.Context, svc *store.DesiredService) ([]string, string) {
		p := rt.PlanReplicaSpread(ctx, *svc)
		return p.Nodes, p.Refusal
	}
}

// spreadCleanup remembers nodes that could not be cleaned, per service. It is in memory only:
// a restart while the node is down forgets it (documented in docs/placement.md).
type spreadCleanup struct {
	mu    sync.Mutex
	nodes map[string]map[string]bool
}

var sharedSpreadCleanup = &spreadCleanup{nodes: map[string]map[string]bool{}}

func (c *spreadCleanup) add(svc, node string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodes[svc] == nil {
		c.nodes[svc] = map[string]bool{}
	}
	c.nodes[svc][node] = true
}

func (c *spreadCleanup) remove(svc, node string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodes[svc], node)
	if len(c.nodes[svc]) == 0 {
		delete(c.nodes, svc)
	}
}

func (c *spreadCleanup) list(svc string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.nodes[svc]))
	for n := range c.nodes[svc] {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// spreadWanted is true when a service must go through the spread controller: it already has a
// spread to unwind, or the live setting is on and it has more than one replica.
func spreadWanted(ctx context.Context, deps dynamicSourceDeps, svc store.DesiredService) bool {
	if len(svc.ReplicaNodes) > 0 || len(sharedSpreadCleanup.list(svc.Name)) > 0 {
		return true
	}
	s := deps.settingsService()
	return s != nil && svc.Replicas > 1 && s.Bool(ctx, api.KeyPlacementReplicaSpread)
}

// offlineSub stands in for a node that has no live transport this pass.
type offlineSub struct{ inner reconcile.Controller }

func (o offlineSub) Reconcile(ctx context.Context) (reconcile.Result, error) {
	return o.inner.Reconcile(ctx)
}
func (offlineSub) Teardown(context.Context) error { return nil }

// noopSub skips a cleanup that would hit a runtime still serving a live replica.
type noopSub struct{}

func (noopSub) Reconcile(context.Context) (reconcile.Result, error) { return reconcile.Result{}, nil }
func (noopSub) Teardown(context.Context) error                      { return nil }

// cleanupSub records an unreachable node for retry and forgets it once a cleanup pass succeeds.
type cleanupSub struct {
	inner       application.SpreadSub
	svc, node   string
	unreachable bool
}

func (c cleanupSub) Reconcile(ctx context.Context) (reconcile.Result, error) {
	if c.unreachable {
		sharedSpreadCleanup.add(c.svc, c.node)
		return c.inner.Reconcile(ctx)
	}
	res, err := c.inner.Reconcile(ctx)
	if err == nil {
		sharedSpreadCleanup.remove(c.svc, c.node)
	}
	return res, err
}

func (c cleanupSub) Teardown(ctx context.Context) error { return c.inner.Teardown(ctx) }

// spreadControllerFor builds the wrapper for one spread service, one sub controller per node.
func spreadControllerFor(ctx context.Context, deps dynamicSourceDeps, svc store.DesiredService, appOpts []application.Option, snap *application.PassSnapshot, plan spreadPlanner) reconcile.Controller {
	var mu sync.Mutex
	active := map[string]bool{}
	sd := application.SpreadDeps{
		Store: deps.db,
		Plan:  plan,
		Commit: func(ctx context.Context, name string, nodes []string) error {
			return deps.db.UpdateServiceReplicaNodes(ctx, name, nodes)
		},
		Pending: func() []string { return sharedSpreadCleanup.list(svc.Name) },
		Build: func(node string, indexes []int, rec application.DeployRecorder) application.SpreadSub {
			nodeID := runtimeNodeID(deps, node)
			cleanup := indexes != nil && len(indexes) == 0
			mu.Lock()
			if cleanup && active[nodeID] {
				// The same runtime serves a live replica, so removing "stale" ones here would kill it.
				mu.Unlock()
				return noopSub{}
			}
			if !cleanup {
				active[nodeID] = true
			}
			mu.Unlock()
			rt, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, nodeID)
			if err != nil {
				deps.logger.Warn("spread: node transport unavailable this pass",
					slog.String("service", svc.Name), slog.String("node_id", node), slog.String("error", err.Error()))
				away := svc
				away.NodeID = node
				off := offlineSub{inner: nodeOfflineControllerFor(ctx, deps, away)}
				if cleanup {
					return cleanupSub{inner: off, svc: svc.Name, node: node, unreachable: true}
				}
				return off
			}
			opts := append(appOpts[:len(appOpts):len(appOpts)],
				application.WithPassSnapshot(snap, nodeID), application.WithImageIDCache(deps.imageIDs, nodeID),
				application.WithNodeRecovering(nodeRecentlyRecovered(ctx, deps, node)))
			if indexes != nil {
				opts = append(opts, application.WithReplicaIndexes(indexes))
			}
			opts = append(opts, application.WithDeployRecorder(rec))
			var sub application.SpreadSub = application.New(svc.Name, deps.db, rt, opts...)
			if cleanup {
				sub = cleanupSub{inner: sub, svc: svc.Name, node: node}
			}
			return sub
		},
	}
	if deps.telemetryDB != nil {
		sd.Recorder = deps.telemetryDB
	}
	ctl := reconcile.Controller(application.NewSpreadController(svc.Name, sd))
	if svc.NodeID != "" {
		ctl = application.WithPendingAttemptResolution(ctl, svc.Name, deps.db)
	}
	return ctl
}

// spreadPrefetch maps each node to the replica containers it should already hold.
func spreadPrefetch(deps dynamicSourceDeps, svc store.DesiredService, targets map[string]*prefetchNode, homeRT docker.Runtime, homeID string) {
	all := replicaTargets(svc)
	if len(svc.ReplicaNodes) != len(all) {
		n := targets[homeID]
		if n == nil {
			n = &prefetchNode{rt: homeRT}
			targets[homeID] = n
		}
		n.names = append(n.names, all...)
		return
	}
	for i, node := range svc.ReplicaNodes {
		id := runtimeNodeID(deps, node)
		n := targets[id]
		if n == nil {
			rt, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, id)
			if err != nil {
				continue
			}
			n = &prefetchNode{rt: rt}
			targets[id] = n
		}
		n.names = append(n.names, all[i])
	}
}
