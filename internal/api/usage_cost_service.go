package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

const (
	usageLookbackEnv      = "APP_USAGE_ROLLUP_LOOKBACK_DAYS"
	usageLiveTTLEnv       = "APP_USAGE_LIVE_CACHE_SECONDS"
	usageDefaultLookback  = 35
	usageDefaultLiveTTL   = 60
	usageMetricCPU        = "cpu_percent"
	usageMetricMemory     = "memory_usage_bytes"
	usageRecentDays       = 7
	usageNodeMemoryWindow = 10 * time.Minute
)

// usageCostStore is the persistence the cost showback needs; *store.DB
// satisfies it.
type usageCostStore interface {
	GetUsageCostSettings(ctx context.Context) (usagecost.Settings, error)
	SaveUsageCostSettings(ctx context.Context, s usagecost.Settings, now time.Time) error
	ListUsageCostNodes(ctx context.Context) (map[string]usagecost.NodePrice, error)
	ReplaceUsageCostNodes(ctx context.Context, nodes []usagecost.NodePrice, now time.Time) error
	SaveUsageDay(ctx context.Context, day string, rows []usagecost.DailyCost) error
	ListUsageDays(ctx context.Context, from, to string) ([]usagecost.DailyCost, error)
	ListUsageBudgets(ctx context.Context) ([]usagecost.Budget, error)
	GetUsageBudget(ctx context.Context, id string) (usagecost.Budget, error)
	SaveUsageBudget(ctx context.Context, b usagecost.Budget, now time.Time) (usagecost.Budget, error)
	DeleteUsageBudget(ctx context.Context, id string) error
}

// usageCostState serialises rollups and caches the partial current day.
type usageCostState struct {
	mu         sync.Mutex
	lastRollup time.Time
	liveAt     time.Time
	live       []usagecost.DailyCost
}

func (rt *Router) usageStore() usageCostStore {
	if s, ok := rt.apps.(usageCostStore); ok {
		return s
	}
	if s, ok := rt.policies.(usageCostStore); ok {
		return s
	}
	return nil
}

func usageEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// usageInventory lists every app and database with its owner, placement
// and environment, as empty usage rows ready to be filled from telemetry.
func (rt *Router) usageInventory(ctx context.Context) ([]usagecost.DailyUsage, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("usage cost: list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("usage cost: list databases: %w", err)
	}
	dbs = rt.withoutPendingDatabases(ctx, dbs)
	var appOwners, dbOwners map[string]*store.ResourceOwner
	if tdb := rt.teamDB(); tdb != nil {
		if appOwners, err = tdb.OwnersOfApps(ctx); err != nil {
			return nil, fmt.Errorf("usage cost: app owners: %w", err)
		}
		if dbOwners, err = tdb.OwnersOfDatabases(ctx); err != nil {
			return nil, fmt.Errorf("usage cost: database owners: %w", err)
		}
	}
	var dbEnvs map[string]*store.EnvironmentRef
	if adb := rt.accessDB(); adb != nil {
		if dbEnvs, err = adb.EnvironmentsOfDatabases(ctx); err != nil {
			return nil, fmt.Errorf("usage cost: database environments: %w", err)
		}
	}
	team := func(m map[string]*store.ResourceOwner, name string) string {
		if o := m[name]; o != nil && o.TeamID != "" {
			return o.TeamID
		}
		return store.DefaultTeamID
	}
	out := make([]usagecost.DailyUsage, 0, len(svcs)+len(dbs))
	for _, s := range svcs {
		out = append(out, usagecost.DailyUsage{
			Kind: usagecost.KindApp, Name: s.Name, TeamID: team(appOwners, s.Name),
			ProjectID: s.ProjectID, EnvironmentID: s.EnvironmentID, NodeID: rt.advisorNodeOf(s.NodeID),
		})
	}
	for _, d := range dbs {
		env := ""
		if ref := dbEnvs[d.Name]; ref != nil {
			env = ref.ID
		}
		out = append(out, usagecost.DailyUsage{
			Kind: usagecost.KindDatabase, Name: d.Name, TeamID: team(dbOwners, d.Name),
			ProjectID: d.ProjectID, EnvironmentID: env, NodeID: rt.advisorNodeOf(d.NodeID),
		})
	}
	return out, nil
}

func usageResourceID(u usagecost.DailyUsage) string {
	if u.Kind == usagecost.KindDatabase {
		return resourceIDForDatabase(u.Name)
	}
	return resourceIDForApp(u.Name)
}

func (rt *Router) usageSamples(ctx context.Context, resourceID, metric string, from, to time.Time) []usagecost.Sample {
	if rt.telemetry == nil {
		return nil
	}
	raw, err := rt.telemetry.QueryMetrics(ctx, resourceID, metric, from, to)
	if err != nil {
		rt.logger.Warn("usage cost: query metric", slog.String("resource", resourceID), slog.String("metric", metric), slog.String("error", err.Error()))
		return nil
	}
	out := make([]usagecost.Sample, len(raw))
	for i, s := range raw {
		out[i] = usagecost.Sample{At: s.Timestamp, Value: s.Value}
	}
	return out
}

// usageStorage sums local named volume bytes per owning resource.
func (rt *Router) usageStorage(ctx context.Context) map[string]float64 {
	out := map[string]float64{}
	var vols volumeUsageResource
	all := func(string) bool { return true }
	if err := rt.fillVolumeUsage(ctx, &vols, all, all); err != nil || vols.NotConfigured {
		return out
	}
	for _, v := range vols.Items {
		if v.SizeBytes == nil || (v.OwnerKind != usageOwnerApp && v.OwnerKind != usageOwnerDatabase) {
			continue
		}
		out[v.OwnerKind+":"+v.Owner] += float64(*v.SizeBytes)
	}
	return out
}

// usageCollectDay reads one window of telemetry for every resource.
func (rt *Router) usageCollectDay(ctx context.Context, bases []usagecost.DailyUsage, from, to time.Time, storage map[string]float64) []usagecost.DailyUsage {
	day := usagecost.DayKey(from)
	out := make([]usagecost.DailyUsage, 0, len(bases))
	for _, b := range bases {
		id := usageResourceID(b)
		b.Day = day
		u := usagecost.UsageFromSamples(b, rt.usageSamples(ctx, id, usageMetricCPU, from, to),
			rt.usageSamples(ctx, id, usageMetricMemory, from, to), from, to)
		if u.Kind == usagecost.KindApp {
			u.EgressBytes = usagecost.SumSamples(rt.usageSamples(ctx, id, telemetry.MetricHTTPBytesOut, from, to), from, to)
		}
		u.StorageBytes = storage[u.Kind+":"+u.Name]
		if u.UpSeconds <= 0 && u.StorageBytes <= 0 && u.EgressBytes <= 0 {
			continue
		}
		out = append(out, u)
	}
	return out
}

// usageNodeCapacity is what the platform can detect about a node without
// operator input: local cores and memory, or the reported total memory.
func (rt *Router) usageNodeCapacity(ctx context.Context, nodeID string, now time.Time) (vcpus, memGiB float64) {
	if nodeID == rt.advisorNodeOf("") {
		cores := rt.doctorNumCPU
		if cores == nil {
			cores = runtime.NumCPU
		}
		vcpus = float64(cores())
		if total, _, err := telemetry.HostMemoryBytes(); err == nil {
			memGiB = float64(total) / (1 << 30)
		}
		return vcpus, memGiB
	}
	if rt.telemetry != nil {
		samples, err := rt.telemetry.QueryMetrics(ctx, nodeResourceID(nodeID), telemetry.MetricMemoryTotalBytes, now.Add(-usageNodeMemoryWindow), now)
		if err == nil && len(samples) > 0 {
			memGiB = samples[len(samples)-1].Value / (1 << 30)
		}
	}
	return vcpus, memGiB
}

// usageNodePrices returns the configured node bills with any missing
// capacity filled from what the platform detects.
func (rt *Router) usageNodePrices(ctx context.Context, st usageCostStore, now time.Time) (map[string]usagecost.NodePrice, error) {
	configured, err := st.ListUsageCostNodes(ctx)
	if err != nil {
		return nil, err
	}
	for id, n := range configured {
		if n.VCPUs > 0 && n.MemoryGiB > 0 {
			continue
		}
		v, m := rt.usageNodeCapacity(ctx, id, now)
		if n.VCPUs <= 0 {
			n.VCPUs = v
		}
		if n.MemoryGiB <= 0 {
			n.MemoryGiB = m
		}
		configured[id] = n
	}
	return configured, nil
}

func usageMarker(day string) usagecost.DailyCost {
	return usagecost.DailyCost{DailyUsage: usagecost.DailyUsage{Day: day, Kind: usagecost.KindDay}}
}

// usageRollup snapshots every complete UTC day inside the lookback window
// that has no snapshot yet, pricing it with the settings in force now.
func (rt *Router) usageRollup(ctx context.Context, now time.Time) error {
	st := rt.usageStore()
	if st == nil || rt.telemetry == nil {
		return nil
	}
	rt.usageCost.mu.Lock()
	defer rt.usageCost.mu.Unlock()
	today := usagecost.StartOfDay(now)
	start := today.AddDate(0, 0, -usageEnvInt(usageLookbackEnv, usageDefaultLookback))
	stored, err := st.ListUsageDays(ctx, usagecost.DayKey(start), usagecost.DayKey(today))
	if err != nil {
		return fmt.Errorf("usage cost: list snapshots: %w", err)
	}
	have := map[string]bool{}
	for _, r := range stored {
		if r.Kind == usagecost.KindDay {
			have[r.Day] = true
		}
	}
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		return fmt.Errorf("usage cost: settings: %w", err)
	}
	nodes, err := rt.usageNodePrices(ctx, st, now)
	if err != nil {
		return fmt.Errorf("usage cost: node prices: %w", err)
	}
	bases, err := rt.usageInventory(ctx)
	if err != nil {
		return err
	}
	storage := rt.usageStorage(ctx)
	for d := start; d.Before(today); d = d.AddDate(0, 0, 1) {
		key := usagecost.DayKey(d)
		if have[key] {
			continue
		}
		usage := rt.usageCollectDay(ctx, bases, d, d.AddDate(0, 0, 1), storage)
		res := usagecost.AllocateDay(usagecost.DayInput{Day: d, Settings: settings, Nodes: nodes, Usage: usage, Fraction: 1})
		if err := st.SaveUsageDay(ctx, key, append(res.Rows, usageMarker(key))); err != nil {
			return fmt.Errorf("usage cost: save day: %w", err)
		}
	}
	rt.usageCost.lastRollup = now
	return nil
}

// UsageRollupTick is the scheduler entry point: it snapshots missing days
// and evaluates nothing else, so a failure only delays history.
func (rt *Router) UsageRollupTick(ctx context.Context) {
	if err := rt.usageRollup(ctx, time.Now()); err != nil {
		rt.logger.Warn("usage cost: rollup failed", slog.String("error", err.Error()))
	}
}

// usageReprice recomputes stored days in [from, to) from their saved usage
// with the current settings, for corrections the operator asks for.
func (rt *Router) usageReprice(ctx context.Context, st usageCostStore, from, to time.Time, now time.Time) (int, error) {
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("usage cost: settings: %w", err)
	}
	nodes, err := rt.usageNodePrices(ctx, st, now)
	if err != nil {
		return 0, fmt.Errorf("usage cost: node prices: %w", err)
	}
	stored, err := st.ListUsageDays(ctx, usagecost.DayKey(from), usagecost.DayKey(to))
	if err != nil {
		return 0, fmt.Errorf("usage cost: list snapshots: %w", err)
	}
	byDay := map[string][]usagecost.DailyUsage{}
	days := map[string]bool{}
	for _, r := range stored {
		days[r.Day] = true
		if r.Kind == usagecost.KindApp || r.Kind == usagecost.KindDatabase {
			byDay[r.Day] = append(byDay[r.Day], r.DailyUsage)
		}
	}
	n := 0
	for day := range days {
		d, err := usagecost.ParseDay(day)
		if err != nil {
			continue
		}
		res := usagecost.AllocateDay(usagecost.DayInput{Day: d, Settings: settings, Nodes: nodes, Usage: byDay[day], Fraction: 1})
		if err := st.SaveUsageDay(ctx, day, append(res.Rows, usageMarker(day))); err != nil {
			return n, fmt.Errorf("usage cost: save day: %w", err)
		}
		n++
	}
	return n, nil
}

// usageLiveToday prices the elapsed part of the current day, cached briefly.
func (rt *Router) usageLiveToday(ctx context.Context, st usageCostStore, now time.Time) ([]usagecost.DailyCost, error) {
	rt.usageCost.mu.Lock()
	defer rt.usageCost.mu.Unlock()
	ttl := time.Duration(usageEnvInt(usageLiveTTLEnv, usageDefaultLiveTTL)) * time.Second
	if !rt.usageCost.liveAt.IsZero() && now.Sub(rt.usageCost.liveAt) < ttl {
		return rt.usageCost.live, nil
	}
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("usage cost: settings: %w", err)
	}
	nodes, err := rt.usageNodePrices(ctx, st, now)
	if err != nil {
		return nil, fmt.Errorf("usage cost: node prices: %w", err)
	}
	bases, err := rt.usageInventory(ctx)
	if err != nil {
		return nil, err
	}
	start := usagecost.StartOfDay(now)
	usage := rt.usageCollectDay(ctx, bases, start, now, rt.usageStorage(ctx))
	frac := now.Sub(start).Seconds() / 86400
	res := usagecost.AllocateDay(usagecost.DayInput{Day: start, Settings: settings, Nodes: nodes, Usage: usage, Fraction: frac})
	rt.usageCost.live, rt.usageCost.liveAt = res.Rows, now
	return res.Rows, nil
}

// usageRows returns allocated rows for [from, to): snapshots for complete
// days, and the live partial day when the range reaches it.
func (rt *Router) usageRows(ctx context.Context, st usageCostStore, from, to, now time.Time) ([]usagecost.DailyCost, error) {
	today := usagecost.StartOfDay(now)
	end := to
	if end.After(today) {
		end = today
	}
	var rows []usagecost.DailyCost
	if end.After(from) {
		stored, err := st.ListUsageDays(ctx, usagecost.DayKey(from), usagecost.DayKey(end))
		if err != nil {
			return nil, fmt.Errorf("usage cost: list snapshots: %w", err)
		}
		rows = stored
	}
	if to.After(today) && !from.After(today) && rt.telemetry != nil {
		live, err := rt.usageLiveToday(ctx, st, now)
		if err != nil {
			return nil, err
		}
		rows = append(rows, live...)
	}
	return rows, nil
}

// usageEnsureFresh runs a rollup when snapshots may be missing, without
// failing the read: a stale history is still a valid answer.
func (rt *Router) usageEnsureFresh(ctx context.Context, now time.Time) {
	rt.usageCost.mu.Lock()
	recent := !rt.usageCost.lastRollup.IsZero() && now.Sub(rt.usageCost.lastRollup) < time.Hour
	rt.usageCost.mu.Unlock()
	if recent {
		return
	}
	if err := rt.usageRollup(ctx, now); err != nil {
		rt.logger.Warn("usage cost: on-demand rollup failed", slog.String("error", err.Error()))
	}
}

// BudgetStatuses returns every budget's month to date position, for the
// budget_threshold alert rule.
func (rt *Router) BudgetStatuses(ctx context.Context) ([]usagecost.BudgetStatus, error) {
	st := rt.usageStore()
	if st == nil {
		return nil, nil
	}
	budgets, err := st.ListUsageBudgets(ctx)
	if err != nil || len(budgets) == 0 {
		return nil, err
	}
	return rt.budgetStatuses(ctx, st, budgets, time.Now())
}

func (rt *Router) budgetStatuses(ctx context.Context, st usageCostStore, budgets []usagecost.Budget, now time.Time) ([]usagecost.BudgetStatus, error) {
	rt.usageEnsureFresh(ctx, now)
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("usage cost: settings: %w", err)
	}
	monthStart := usagecost.StartOfMonth(now)
	rows, err := rt.usageRows(ctx, st, monthStart, monthStart.AddDate(0, 1, 0), now)
	if err != nil {
		return nil, err
	}
	today := usagecost.DayKey(usagecost.StartOfDay(now))
	spent := map[string]float64{}
	daily := map[string]map[string]float64{}
	for _, r := range rows {
		if r.Kind != usagecost.KindApp && r.Kind != usagecost.KindDatabase {
			continue
		}
		for _, key := range []string{usagecost.ScopeProject + ":" + r.ProjectID, usagecost.ScopeTeam + ":" + r.TeamID} {
			spent[key] += r.Total()
			if r.Day != today {
				if daily[key] == nil {
					daily[key] = map[string]float64{}
				}
				daily[key][r.Day] += r.Total()
			}
		}
	}
	names := rt.usageScopeNames(ctx)
	elapsed := usagecost.ElapsedDays(now)
	dim := usagecost.DaysInMonth(now)
	out := make([]usagecost.BudgetStatus, 0, len(budgets))
	for _, b := range budgets {
		key := b.Scope + ":" + b.ScopeID
		forecast := spent[key]
		if elapsed >= 1 {
			forecast = usagecost.ForecastMonthEnd(spent[key], elapsed, dim, recentDaily(daily[key], now))
		}
		name := names[key]
		if name == "" {
			name = b.ScopeID
		}
		out = append(out, usagecost.NewStatus(b, name, settings.Currency, spent[key], forecast))
	}
	return out, nil
}

func recentDaily(byDay map[string]float64, now time.Time) []float64 {
	var out []float64
	today := usagecost.StartOfDay(now)
	monthStart := usagecost.StartOfMonth(now)
	for i := 1; i <= usageRecentDays; i++ {
		d := today.AddDate(0, 0, -i)
		if d.Before(monthStart) {
			break
		}
		out = append(out, byDay[usagecost.DayKey(d)])
	}
	return out
}

// usageScopeNames maps "project:<id>" and "team:<id>" to display names.
func (rt *Router) usageScopeNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	if rt.projects != nil {
		if ps, err := rt.projects.ListProjects(ctx); err == nil {
			for _, p := range ps {
				out[usagecost.ScopeProject+":"+p.ID] = p.Name
			}
		}
	}
	if rt.organizations != nil {
		if orgs, err := rt.organizations.ListOrganizations(ctx); err == nil {
			for _, o := range orgs {
				out[usagecost.ScopeTeam+":"+o.ID] = o.Name
			}
		}
	}
	return out
}
