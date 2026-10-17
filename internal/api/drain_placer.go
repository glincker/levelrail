package api

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/placement"
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
	cands      []placement.Candidate
	counts     map[string]int
	planner    *gpuPlanner
	autoPlaced bool
	pending    func(context.Context)
}

// commit records the last next() pick as occupied. Call it only after the
// placement update succeeded, so a failed move holds no capacity.
func (p *drainPlacer) commit(ctx context.Context) {
	if p.pending != nil {
		p.pending(ctx)
		p.pending = nil
	}
}

func (p *drainPlacer) load(ctx context.Context) error {
	if p.loaded {
		return nil
	}
	inv, err := p.rt.loadPlacementInventory(ctx)
	if err != nil {
		return err
	}
	cands, err := p.rt.candidatesFrom(ctx, inv)
	if err != nil {
		return err
	}
	planner, err := p.rt.newGPUPlanner(ctx)
	if err != nil {
		return err
	}
	p.nodes, p.counts, p.cands, p.planner, p.loaded = inv.nodes, inv.counts, cands, planner, true
	return nil
}

// drainRef names the resource being placed and what it asks for.
type drainRef struct {
	kind  string
	name  string
	needs placement.Resources
}

// next returns the node for one resource. claim is nil unless the
// resource asks for GPUs; a *errNoGPUTarget means it must stay put. A
// resource no node has room for returns an error naming each shortfall.
func (p *drainPlacer) next(ctx context.Context, claim *gpu.Claim, ref drainRef) (string, error) {
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
	d, err := placement.Spread(p.cands, placement.Request{Exclude: p.fromID, Needs: ref.needs, Policy: p.rt.placementPolicy})
	if err != nil {
		if d.CapacityBlocked() {
			return "", err
		}
		return "", nil
	}
	picked := d.NodeID
	p.pending = func(ctx context.Context) {
		p.counts[picked]++
		p.autoPlaced = true
		for i := range p.cands {
			if p.cands[i].ID == picked {
				p.cands[i].Load++
				p.cands[i].Requested = p.cands[i].Requested.Add(ref.needs)
			}
		}
		p.rt.recordPlacement(ctx, ref.kind, ref.name, placementTriggerDrain, d)
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
	p.pending = func(context.Context) {
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
