package api

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

// drainBlocked is an app a drain could not move.
type drainBlocked struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// drainPlacer resolves the destination of each resource in one drain: the
// explicit target when given, else simple spread scheduling. GPU claims
// are checked against free GPUs, and a claim is reserved only once the
// caller confirms the move with commit.
type drainPlacer struct {
	rt       *Router
	fromID   string
	explicit bool
	target   string

	loaded     bool
	nodes      []store.Node
	counts     map[string]int
	planner    *gpuPlanner
	autoPlaced bool
	pending    func()
}

// commit records the last next() pick as occupied. Call it only after the
// placement update succeeded, so a failed move holds no capacity.
func (p *drainPlacer) commit() {
	if p.pending != nil {
		p.pending()
		p.pending = nil
	}
}

func (p *drainPlacer) load(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	nodes, counts, err := p.rt.placementCounts(ctx)
	if err != nil {
		return err
	}
	planner, err := p.rt.newGPUPlanner(ctx)
	if err != nil {
		return err
	}
	p.nodes, p.counts, p.planner, p.loaded = nodes, counts, planner, true
	return nil
}

// next returns the node for one resource. claim is nil unless the
// resource asks for GPUs; a *errNoGPUTarget means it must stay put.
func (p *drainPlacer) next(ctx context.Context, claim *gpu.Claim) (string, error) {
	p.pending = nil
	if claim != nil && p.rt.models != nil {
		return p.nextGPU(ctx, *claim)
	}
	if p.explicit {
		return p.target, nil
	}
	if !p.rt.autoPlacementEnabled {
		return "", nil
	}
	if err := p.load(ctx); err != nil {
		return "", err
	}
	picked := selectLeastLoadedNodeExcluding(p.nodes, p.counts, p.fromID)
	if picked != "" {
		p.pending = func() {
			p.counts[picked]++
			p.autoPlaced = true
		}
	}
	return picked, nil
}

func (p *drainPlacer) nextGPU(ctx context.Context, claim gpu.Claim) (string, error) {
	if err := p.load(ctx); err != nil {
		return "", err
	}
	var (
		target string
		auto   bool
		err    error
	)
	switch {
	case p.explicit:
		if fitErr := p.planner.fit(p.target, claim); fitErr != nil {
			return "", &errNoGPUTarget{reason: "no GPU node available (" + p.planner.label(p.target) + ": " + fitErr.Error() + ")"}
		}
		target = p.target
	case !p.rt.autoPlacementEnabled:
		if fitErr := p.planner.fit("", claim); fitErr != nil {
			return "", &errNoGPUTarget{reason: "no GPU node available (" + p.planner.label("") + ": " + fitErr.Error() + ")"}
		}
	default:
		target, err = p.planner.spread(p.nodes, p.counts, p.fromID, claim)
		if err != nil {
			return "", err
		}
		auto = true
	}
	p.pending = func() {
		if auto {
			p.counts[target]++
			p.autoPlaced = p.autoPlaced || target != ""
		}
		p.planner.reserve(target, claim)
	}
	return target, nil
}

// drainModelBlocks lists models pinned to the node: a model has no move
// operation, so a drain reports them instead of dropping them silently.
func (rt *Router) drainModelBlocks(ctx context.Context, nodeID string) ([]drainBlocked, error) {
	if rt.models == nil {
		return nil, nil
	}
	views, err := rt.models.List(ctx)
	if err != nil {
		rt.logger.Error("api: drain node: list models failed", "error", err.Error(), "node_id", nodeID)
		return nil, fmt.Errorf("list models: %w", err)
	}
	var out []drainBlocked
	for _, v := range views {
		if v.Model.NodeID == nodeID && !v.Model.Deleting {
			out = append(out, drainBlocked{Kind: "model", Name: v.Model.Name, Reason: "models cannot be moved; delete it and deploy it on another GPU node with --node"})
		}
	}
	return out, nil
}
