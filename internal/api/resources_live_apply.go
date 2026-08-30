package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyResourcesLive pushes resources onto containerName's currently
// running container via the Engine API's live ContainerUpdate, instead
// of waiting for a recreate. Best effort: nodeID not reachable, or the
// container not currently running, both return false with no error
// surfaced to the caller. The existing recreate-on-next-deploy-or-
// restart path still converges resources correctly regardless, so this
// is purely a fast path, never the only way resources take effect.
func (rt *Router) applyResourcesLive(ctx context.Context, nodeID, containerName string, resources *store.ServiceResources) bool {
	if rt.execRuntime == nil {
		return false
	}
	runtime, err := rt.execRuntime(nodeID)
	if err != nil {
		return false
	}
	state, err := runtime.InspectByName(ctx, containerName)
	if err != nil || state == nil || !state.Running {
		return false
	}
	var toApply docker.Resources
	if resources != nil {
		toApply = docker.Resources{
			MemoryBytes:     resources.MemoryBytes,
			NanoCPUs:        resources.NanoCPUs,
			SwapMemoryBytes: resources.SwapMemoryBytes,
			CPUSetCPUs:      resources.CPUSetCPUs,
		}
	}
	return runtime.UpdateResources(ctx, state.ID, toApply) == nil
}

// applyResourcesLiveToReplicas is applyResourcesLive generalized to
// every replica target desired.Replicas describes (desiredServiceContainerNames,
// system_prune.go), for a multi-replica app. Returns true if it applied
// live to at least one running replica; a partially-converged app (some
// replicas updated, others not yet running) still reports true, since
// every replica that is running now has the right limits, and any
// replica not yet running will get them at create time regardless.
func (rt *Router) applyResourcesLiveToReplicas(ctx context.Context, desired store.DesiredService) bool {
	applied := false
	for _, name := range desiredServiceContainerNames(desired) {
		if rt.applyResourcesLive(ctx, desired.NodeID, name, desired.Resources) {
			applied = true
		}
	}
	return applied
}
