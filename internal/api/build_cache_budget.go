package api

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/build/cachebudget"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// BuildCacheBudgeter is the optional surface for the post build cache budget.
// *build.Client satisfies it.
type BuildCacheBudgeter interface {
	CacheBudgetEngine(ctx context.Context) (build.CacheBudgetEngine, error)
	PruneCacheToBudget(ctx context.Context, b build.CacheBudget) (build.CacheBudgetPrune, error)
}

// cacheBudgetPlan is the resolved budget for one engine reading.
type cacheBudgetPlan struct {
	Enabled bool
	Reason  string
	Budget  build.CacheBudget
	Engine  build.CacheBudgetEngine
}

// cacheBudgetGOOS is the platform auto mode checks; tests replace it.
var cacheBudgetGOOS = runtime.GOOS

func (rt *Router) cacheBudgeter() (BuildCacheBudgeter, bool) {
	if rt.appBuildCache == nil {
		return nil, false
	}
	b, ok := rt.appBuildCache.(BuildCacheBudgeter)
	return b, ok
}

// planCacheBudget resolves the live mode and size. Auto stays off off Linux and
// on Docker Desktop, where the cache lives in a VM disk sized by the user.
func (rt *Router) planCacheBudget(ctx context.Context, b BuildCacheBudgeter) (cacheBudgetPlan, error) {
	eng, err := b.CacheBudgetEngine(ctx)
	if err != nil {
		return cacheBudgetPlan{}, err
	}
	plan := cacheBudgetPlan{Engine: eng}
	mode := runtimesettings.BuildCacheBudgetAuto
	var maxMB, reservedMB int
	if rt.runtimeSettings != nil {
		mode = rt.runtimeSettings.String(ctx, runtimesettings.KeyBuildCacheBudget)
		maxMB = rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheBudgetMaxMB)
		reservedMB = rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildCacheBudgetReservedMB)
	}
	switch {
	case mode == runtimesettings.BuildCacheBudgetOff:
		plan.Reason = "off by the build.cache_budget setting"
	case mode == runtimesettings.BuildCacheBudgetOn:
		plan.Enabled = true
	case cacheBudgetGOOS != "linux":
		plan.Reason = "auto is off on " + cacheBudgetGOOS
	case eng.Desktop:
		plan.Reason = "auto is off on Docker Desktop"
	default:
		plan.Enabled = true
	}
	var total int64
	if eng.RootDir != "" {
		if t, _, derr := telemetry.HostDiskBytes(eng.RootDir); derr == nil {
			total = t
		}
	}
	plan.Budget = build.CacheBudget(cachebudget.BudgetFor(total, int64(reservedMB), int64(maxMB)))
	return plan, nil
}

// pruneCacheToBudget runs after each build; it shares the scan gate with the
// per app quota so only one disk usage scan runs at a time.
func (rt *Router) pruneCacheToBudget(ctx context.Context) {
	b, ok := rt.cacheBudgeter()
	if !ok {
		return
	}
	select {
	case cacheScanGate <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-cacheScanGate }()
	plan, err := rt.planCacheBudget(ctx, b)
	if err != nil {
		rt.logger.Warn("api: build cache budget: read usage failed", slog.String("error", err.Error()))
		return
	}
	if !plan.Enabled || plan.Engine.UsedBytes <= plan.Budget.MaxUsedBytes {
		return
	}
	res, err := b.PruneCacheToBudget(ctx, plan.Budget)
	if err != nil {
		rt.logger.Warn("api: build cache budget prune failed", slog.String("error", err.Error()), slog.Int64("freed_bytes", res.FreedBytes))
		return
	}
	rt.logger.Info("api: build cache pruned to budget", slog.Int64("used_bytes", plan.Engine.UsedBytes),
		slog.Int64("budget_bytes", plan.Budget.MaxUsedBytes), slog.Int64("freed_bytes", res.FreedBytes), slog.Int("removed", res.Removed))
}

// doctorCheckBuildCacheBudget shows the builder cache size against its budget.
func (rt *Router) doctorCheckBuildCacheBudget(ctx context.Context) doctorCheckResource {
	const code, name = "build_cache_budget", "Build cache budget"
	b, ok := rt.cacheBudgeter()
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "No BuildKit builder is connected, so the build cache size was not read."}
	}
	plan, err := rt.planCacheBudget(ctx, b)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "The build cache size could not be read."}
	}
	used, budget := mibOf(plan.Engine.UsedBytes), mibOf(plan.Budget.MaxUsedBytes)
	if !plan.Enabled {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK,
			Message: fmt.Sprintf("Build cache uses %d MiB; the budget of %d MiB is not applied (%s)", used, budget, plan.Reason)}
	}
	if plan.Engine.UsedBytes > plan.Budget.MaxUsedBytes {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn,
			Message: fmt.Sprintf("Build cache uses %d MiB, over its %d MiB budget; the next finished build prunes it", used, budget),
			Fix:     "Raise build.cache_budget_max_mb if the disk can spare it, or set build.cache_budget to off to keep everything."}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK,
		Message: fmt.Sprintf("Build cache uses %d MiB of its %d MiB budget", used, budget)}
}

func mibOf(b int64) int64 { return b >> 20 }
