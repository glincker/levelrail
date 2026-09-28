package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// NodeGPUChecker reports a node's GPU capability. nodeID "" is the local
// node.
type NodeGPUChecker interface {
	NodeGPU(ctx context.Context, nodeID string) (gpu.Info, error)
}

// WithNodeGPU makes a service that requests resources.gpu refuse to
// start on a node without a usable GPU.
func WithNodeGPU(checker NodeGPUChecker) Option {
	return func(c *Controller) { c.nodeGPU = checker }
}

// gpuPlacementBlock returns a not-ready result when desired asks for a GPU
// its node cannot provide, nil otherwise.
func (c *Controller) gpuPlacementBlock(ctx context.Context, desired *store.DesiredService) *reconcile.Result {
	if desired.Resources == nil || desired.Resources.GPU == nil || c.nodeGPU == nil {
		return nil
	}
	info, err := c.nodeGPU.NodeGPU(ctx, desired.NodeID)
	if err != nil {
		res := notReady("GPUCheckFailed", err)
		return &res
	}
	switch {
	case !info.Present:
		res := notReady("NoGPUOnNode", fmt.Errorf("service %q requests a GPU but its node reports none; move it to a GPU node", c.serviceName))
		return &res
	case !info.RuntimeInstalled:
		res := notReady("GPURuntimeMissing", fmt.Errorf("node has a GPU but Docker has no nvidia runtime. %s", gpu.InstallHint))
		return &res
	}
	return nil
}
