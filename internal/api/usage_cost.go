package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

const (
	usageMaxCustomDays = 366
	usageDefaultWindow = 30
	usageNotConfigured = "cost showback storage is not configured"
)

var usageCurrencyRe = regexp.MustCompile(`^[A-Za-z]{3,8}$`)

type usageReportResponse struct {
	usagecost.Report
	Currency            string            `json:"currency"`
	Mode                string            `json:"mode"`
	Basis               string            `json:"basis"`
	Labels              map[string]string `json:"labels"`
	UnpricedNodes       []string          `json:"unpriced_nodes"`
	IncludesUnallocated bool              `json:"includes_unallocated"`
	Configured          bool              `json:"configured"`
	TelemetryAvailable  bool              `json:"telemetry_available"`
	GeneratedAt         string            `json:"generated_at"`
}

// parseUsagePeriod resolves ?period= (mtd, 30d, last_month, custom with
// from and to, both inclusive days) into a half open range.
func parseUsagePeriod(q map[string][]string, now time.Time) (usagecost.Period, usagecost.Period, error) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	tomorrow := usagecost.StartOfDay(now).AddDate(0, 0, 1)
	var p usagecost.Period
	mtd := false
	switch get("period") {
	case "", "mtd":
		p = usagecost.Period{From: usagecost.StartOfMonth(now), To: tomorrow}
		mtd = true
	case "30d":
		p = usagecost.Period{From: tomorrow.AddDate(0, 0, -usageDefaultWindow), To: tomorrow}
	case "last_month":
		start := usagecost.StartOfMonth(now)
		p = usagecost.Period{From: start.AddDate(0, -1, 0), To: start}
	case "custom":
		from, err := usagecost.ParseDay(get("from"))
		if err != nil {
			return p, p, errors.New("from must be a YYYY-MM-DD day")
		}
		to, err := usagecost.ParseDay(get("to"))
		if err != nil {
			return p, p, errors.New("to must be a YYYY-MM-DD day")
		}
		p = usagecost.Period{From: from, To: to.AddDate(0, 0, 1)}
		if !p.To.After(p.From) || p.Days() > usageMaxCustomDays {
			return p, p, fmt.Errorf("custom range must be 1 to %d days", usageMaxCustomDays)
		}
	default:
		return p, p, errors.New("period must be mtd, 30d, last_month or custom")
	}
	return p, usagecost.PreviousPeriod(p, mtd), nil
}

// usageRowFilter returns which rows the caller may see and whether the
// unallocated remainder (an instance wide figure) is one of them.
func (rt *Router) usageRowFilter(r *http.Request, inventory []usagecost.DailyUsage) (func(usagecost.DailyCost) bool, bool, error) {
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return nil, false, err
	}
	exists := map[string]bool{}
	for _, u := range inventory {
		exists[u.Kind+":"+u.Name] = true
	}
	seesIdle := !scope.restricted()
	return func(row usagecost.DailyCost) bool {
		switch row.Kind {
		case usagecost.KindIdle:
			return seesIdle
		case usagecost.KindApp, usagecost.KindDatabase:
			prefix := resourcePrefixApp
			if row.Kind == usagecost.KindDatabase {
				prefix = resourcePrefixDatabase
			}
			if exists[row.Kind+":"+row.Name] {
				return scope.authorize(abilities, policies, AbilityRead, prefix, row.Name)
			}
			return !scope.vis.Restricted && scope.teams.allowsProject(row.TeamID, row.ProjectID)
		}
		return false
	}, seesIdle, nil
}

func filterUsageRows(rows []usagecost.DailyCost, keep func(usagecost.DailyCost) bool) []usagecost.DailyCost {
	out := rows[:0:0]
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

type usageEnvLister interface {
	ListAllEnvironments(ctx context.Context) ([]store.EnvironmentWithCounts, error)
}

func (rt *Router) usageLabels(ctx context.Context) map[string]string {
	out := rt.usageScopeNames(ctx)
	var lister usageEnvLister
	if l, ok := rt.apps.(usageEnvLister); ok {
		lister = l
	} else if l, ok := rt.policies.(usageEnvLister); ok {
		lister = l
	}
	if lister != nil {
		if envs, err := lister.ListAllEnvironments(ctx); err == nil {
			for _, e := range envs {
				out["environment:"+e.ID] = e.Name
			}
		}
	}
	return out
}

// handleUsageReport handles GET /api/v1/usage/report.
func (rt *Router) handleUsageReport(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	ctx := r.Context()
	now := time.Now()
	period, prevPeriod, err := parseUsagePeriod(r.URL.Query(), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.usageEnsureFresh(ctx, now)
	inv, err := rt.usageInventory(ctx)
	if err != nil {
		rt.internalError(w, "api: usage report: inventory", err)
		return
	}
	keep, seesIdle, err := rt.usageRowFilter(r, inv)
	if err != nil {
		rt.internalError(w, "api: usage report: visibility", err)
		return
	}
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: usage report: settings", err)
		return
	}
	rows, err := rt.usageRows(ctx, st, period.From, period.To, now)
	if err != nil {
		rt.internalError(w, "api: usage report: rows", err)
		return
	}
	prev, err := rt.usageRows(ctx, st, prevPeriod.From, prevPeriod.To, now)
	if err != nil {
		rt.internalError(w, "api: usage report: previous rows", err)
		return
	}
	rows, prev = filterUsageRows(rows, keep), filterUsageRows(prev, keep)
	report := usagecost.Rollup(period, prevPeriod, rows, prev, seesIdle)
	report.Tags = usagecost.RollupTags(period, prevPeriod, rows, prev, rt.tagNamesByResource(ctx))
	resp := usageReportResponse{
		Report: report, Currency: settings.Currency, Mode: settings.Mode, Basis: settings.Basis,
		Labels: visibleUsageLabels(report, rt.usageLabels(ctx)), UnpricedNodes: []string{}, IncludesUnallocated: seesIdle,
		TelemetryAvailable: rt.telemetry != nil, GeneratedAt: now.UTC().Format(time.RFC3339),
	}
	resp.Forecast = usageForecast(period, rows, now)
	nodes, err := rt.usageNodePrices(ctx, st, now)
	if err != nil {
		rt.internalError(w, "api: usage report: nodes", err)
		return
	}
	resp.Configured = usageConfigured(settings, nodes)
	if seesIdle {
		resp.UnpricedNodes = usageUnpricedNodes(settings, nodes, rows)
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="usage-report-%s_%s.csv"`,
			usagecost.DayKey(period.From), usagecost.DayKey(period.To.AddDate(0, 0, -1))))
		if err := usagecost.WriteCSV(w, settings.Currency, report, resp.Labels); err != nil {
			rt.logger.Warn("api: usage report: write csv: " + err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// visibleUsageLabels keeps only the names of groups present in the report,
// so a team scoped caller never learns another team's project names.
func visibleUsageLabels(r usagecost.Report, all map[string]string) map[string]string {
	out := map[string]string{}
	for dim, groups := range map[string][]usagecost.Group{
		usagecost.DimProject: r.Projects, usagecost.DimEnvironment: r.Environments, usagecost.DimTeam: r.Teams,
	} {
		for _, g := range groups {
			if name, ok := all[dim+":"+g.Key]; ok {
				out[dim+":"+g.Key] = name
			}
		}
	}
	return out
}

func usageConfigured(s usagecost.Settings, nodes map[string]usagecost.NodePrice) bool {
	if s.Mode == usagecost.ModeNode {
		for _, n := range nodes {
			if n.Priced() {
				return true
			}
		}
		return false
	}
	return s.CPUCoreHour > 0 || s.MemGiBHour > 0 || s.StorageGiBMonth > 0 || s.EgressGiB > 0
}

func usageUnpricedNodes(s usagecost.Settings, nodes map[string]usagecost.NodePrice, rows []usagecost.DailyCost) []string {
	out := []string{}
	if s.Mode != usagecost.ModeNode {
		return out
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if (r.Kind != usagecost.KindApp && r.Kind != usagecost.KindDatabase) || r.UpSeconds <= 0 || seen[r.NodeID] {
			continue
		}
		if n, ok := nodes[r.NodeID]; !ok || !n.Priced() {
			seen[r.NodeID] = true
			out = append(out, r.NodeID)
		}
	}
	sort.Strings(out)
	return out
}

// usageForecast projects the month end for a month to date window.
func usageForecast(p usagecost.Period, rows []usagecost.DailyCost, now time.Time) *float64 {
	if !p.From.Equal(usagecost.StartOfMonth(now)) || p.To.Before(usagecost.StartOfDay(now).AddDate(0, 0, 1)) {
		return nil
	}
	var spent float64
	byDay := map[string]float64{}
	today := usagecost.DayKey(usagecost.StartOfDay(now))
	for _, r := range rows {
		spent += r.Total()
		if r.Day != today {
			byDay[r.Day] += r.Total()
		}
	}
	elapsed := usagecost.ElapsedDays(now)
	f := spent
	if elapsed >= 1 {
		f = usagecost.ForecastMonthEnd(spent, elapsed, usagecost.DaysInMonth(now), recentDaily(byDay, now))
	}
	return &f
}
