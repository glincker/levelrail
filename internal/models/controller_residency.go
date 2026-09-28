package models

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// residency handles an on-demand model before the normal converge path.
// It returns done=true when the pass is fully handled (idle, or waiting
// for VRAM). Otherwise the model should run and m carries the state.
func (c *Controller) residency(ctx context.Context, m *store.Model, node NodeInfo, state *docker.ContainerState) (reconcile.Result, bool, error) {
	now := c.now()
	if !ResidencyActive(*m, now) {
		return c.sleep(ctx, m, node, state, now)
	}
	if m.ResidencyState != store.ResidencyAsleep {
		return reconcile.Result{}, false, nil
	}
	if blocked := c.waitForGPUWithSwap(ctx, m, node); blocked != nil {
		return *blocked, true, nil
	}
	if err := c.store.SetModelResidencyState(ctx, m.Name, store.ResidencyWaking); err != nil {
		return notReady("StoreError", err.Error()), true, fmt.Errorf("models/%s: mark waking: %w", c.name, err)
	}
	m.ResidencyState = store.ResidencyWaking
	return reconcile.Result{}, false, nil
}

func (c *Controller) sleep(ctx context.Context, m *store.Model, node NodeInfo, state *docker.ContainerState, now time.Time) (reconcile.Result, bool, error) {
	// A request may have landed after this pass read the model; the fresh
	// row wins so the engine is not stopped under it.
	if fresh, err := c.store.GetModel(ctx, m.Name); err == nil && ResidencyActive(*fresh, now) {
		*m = *fresh
		return c.residency(ctx, m, node, state)
	}
	if state != nil && state.Running {
		if err := c.runtime.Stop(ctx, state.ID, stopTimeout); err != nil {
			return notReady("StopFailed", err.Error()), true, fmt.Errorf("models/%s: stop idle engine: %w", c.name, err)
		}
	}
	if m.ResidencyState != store.ResidencyAsleep {
		if err := c.store.SetModelResidencyState(ctx, m.Name, store.ResidencyAsleep); err != nil {
			return notReady("StoreError", err.Error()), true, fmt.Errorf("models/%s: mark asleep: %w", c.name, err)
		}
		m.ResidencyState = store.ResidencyAsleep
	}
	return notReady(reasonIdle, fmt.Sprintf("engine stopped after %s idle; it starts on the first request", IdleTTL(*m).Round(time.Second))), true, nil
}

// waitingForGPU blocks a wake when the model would fit an empty GPU set
// but other workloads hold too much VRAM right now. An oversized model
// is not held back: waiting would never help.
func waitingForGPU(m *store.Model, node NodeInfo) *reconcile.Result {
	devs := selectFitDevices(node.GPU, m.GPUCount, m.GPUDeviceIDs)
	if len(devs) == 0 {
		return nil
	}
	in := FitInput{Engine: m.Engine, ModelRef: m.ModelRef, Quantization: m.Quantization, ContextLength: m.ContextLength,
		GPUs: len(devs), MinFreeBytes: -1}
	var total int64
	for _, d := range devs {
		free := deviceFree(d)
		in.FreeBytes += free
		total += d.VRAMTotalMiB * mib
		if in.MinFreeBytes < 0 || free < in.MinFreeBytes {
			in.MinFreeBytes, in.MinTotalBytes = free, d.VRAMTotalMiB*mib
		}
	}
	cfg, params := LoadFitConfig(), LoadFitParams()
	got := cfg.Check(params, in)
	if got.Verdict != FitWontFit || float64(got.NeedBytes) > float64(total)*cfg.FitPercent/100 {
		return nil
	}
	res := notReady(reasonWaitingOnGPU, "waiting for other workloads to free VRAM: "+got.Arithmetic)
	return &res
}

// waitForGPUWithSwap wraps waitingForGPU with swap group eviction: when
// m does not fit and belongs to a non-empty swap_group, it stops the
// group's current resident sibling on the same node (if any) and
// re-checks fit against the freed VRAM. Locked per swap_group so two
// models in the same group can never race each other's eviction
// decision (see swap_lock.go).
func (c *Controller) waitForGPUWithSwap(ctx context.Context, m *store.Model, node NodeInfo) *reconcile.Result {
	blocked := waitingForGPU(m, node)
	if blocked == nil || m.SwapGroup == "" {
		return blocked
	}

	unlock := lockSwapGroup(m.SwapGroup)
	defer unlock()

	evicted, err := c.evictSwapGroupSibling(ctx, m)
	if err != nil {
		res := notReady(reasonWaitingOnGPU, "evict swap group sibling: "+err.Error())
		return &res
	}
	if evicted == "" {
		return blocked
	}

	fresh, err := c.nodes.NodeInfo(ctx, m.NodeID)
	if err != nil {
		return blocked
	}
	return waitingForGPU(m, fresh)
}

// evictSwapGroupSibling stops the first resident (awake or waking)
// sibling of m's own swap_group on the same node, marking it asleep so
// the reconciler leaves it stopped rather than restarting it next pass.
// Returns the evicted model's name, or "" if no evictable sibling was
// found. Best effort per sibling: a sibling whose containers cannot be
// listed is skipped, not fatal, matching "one broken resource must not
// block others" elsewhere in this codebase.
func (c *Controller) evictSwapGroupSibling(ctx context.Context, m *store.Model) (string, error) {
	siblings, err := c.store.ListModelsInSwapGroup(ctx, m.SwapGroup)
	if err != nil {
		return "", fmt.Errorf("list swap group %q: %w", m.SwapGroup, err)
	}
	for _, s := range siblings {
		if s.Name == m.Name || s.NodeID != m.NodeID {
			continue
		}
		if s.ResidencyState != store.ResidencyAwake && s.ResidencyState != store.ResidencyWaking {
			continue
		}
		all, err := c.runtime.ListByPrefix(ctx, ContainerPrefix(c.prefix, s.Name))
		if err != nil {
			continue
		}
		for _, cs := range all {
			if !cs.Running {
				continue
			}
			if err := c.runtime.Stop(ctx, cs.ID, stopTimeout); err != nil {
				return "", fmt.Errorf("stop sibling %q: %w", s.Name, err)
			}
		}
		if err := c.store.SetModelResidencyState(ctx, s.Name, store.ResidencyAsleep); err != nil {
			return "", fmt.Errorf("mark sibling %q asleep: %w", s.Name, err)
		}
		return s.Name, nil
	}
	return "", nil
}

// finishWake reports WakingUp while a woken engine loads and records the
// model awake once it is ready.
func (c *Controller) finishWake(ctx context.Context, m *store.Model, res reconcile.Result, err error) (reconcile.Result, error) {
	if err != nil || m.Residency != store.ResidencyOnDemand || m.ResidencyState != store.ResidencyWaking || len(res.Conditions) == 0 {
		return res, err
	}
	cond := res.Conditions[0]
	switch {
	case cond.Status == reconcile.ConditionTrue:
		if serr := c.store.SetModelResidencyState(ctx, m.Name, store.ResidencyAwake); serr != nil {
			return res, fmt.Errorf("models/%s: mark awake: %w", c.name, serr)
		}
	case cond.Reason == "Starting" || cond.Reason == "Loading":
		res.Conditions[0].Reason = reasonWakingUp
	}
	return res, nil
}
