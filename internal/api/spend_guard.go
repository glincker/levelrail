package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/spendguard"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// spendForecast is the projected month end position of one app cap.
type spendForecast struct {
	Cost        usagecost.Band `json:"cost"`
	EgressBytes int64          `json:"egress_bytes"`
	Pct         float64        `json:"pct"`
	WillExceed  bool           `json:"will_exceed"`
}

type dryRunResource struct {
	spendcap.Preview
	Scope    string `json:"scope"`
	Target   string `json:"target"`
	Currency string `json:"currency"`
	// Note says what the preview is: nothing here was applied.
	Note string `json:"note"`
}

type overrideRequest struct {
	Reason string `json:"reason"`
}

const dryRunNote = "Preview only. Nothing was stopped, scaled or changed."

func (rt *Router) spendGuard() *spendguard.Guard {
	st, ok := rt.apps.(spendguard.Store)
	if !ok {
		return nil
	}
	return &spendguard.Guard{
		Store: st, Logger: rt.logger,
		ProjectOf: func(ctx context.Context, app string) (string, error) {
			svc, err := rt.apps.GetDesiredService(ctx, app)
			if err != nil {
				return "", fmt.Errorf("load app: %w", err)
			}
			return svc.ProjectID, nil
		},
	}
}

// appForecast projects an app cap to month end from the same rows the cost
// showback uses. A nil result means usage tracking is unavailable.
func (rt *Router) appForecast(ctx context.Context, cp spendcap.Cap, u spendcap.Usage, now time.Time) *spendForecast {
	view, err := rt.loadCostBudgetView(ctx, now)
	if err != nil {
		return nil
	}
	b := costbudget.Budget{Scope: costbudget.ScopeApp, ScopeValue: cp.App, Monthly: cp.MonthlyCostCeiling}
	spent, byDay := view.spend(b)
	elapsed := usagecost.ElapsedDays(now)
	days := usagecost.DaysInMonth(now)
	band := usagecost.Band{Spent: spent, Mid: spent, Low: spent, High: spent, Confidence: usagecost.ConfidenceLow}
	if elapsed >= 1 {
		band = usagecost.ForecastBand(spent, elapsed, days, recentDaily(byDay, now))
	}
	f := &spendForecast{Cost: band}
	if elapsed > 0 {
		f.EgressBytes = int64(float64(u.EgressBytes) / elapsed * float64(days))
	}
	if cp.MonthlyCostCeiling > 0 {
		f.Pct = costbudget.Pct(band.Mid, cp.MonthlyCostCeiling)
	}
	if cp.MonthlyEgressBytes > 0 {
		if p := float64(f.EgressBytes) / float64(cp.MonthlyEgressBytes) * 100; p > f.Pct {
			f.Pct = p
		}
	}
	f.WillExceed = f.Pct >= 100
	return f
}

// pauseLoads gathers what stopping each named app would remove: compute and
// egress cost month to date and per recent day. Storage and idle node cost stay.
func (rt *Router) pauseLoads(ctx context.Context, view costBudgetView, apps []string) []spendcap.AppLoad {
	want := map[string]bool{}
	for _, a := range apps {
		want[a] = true
	}
	today := usagecost.DayKey(usagecost.StartOfDay(view.now))
	mtd := map[string]float64{}
	byDay := map[string]map[string]float64{}
	for _, r := range view.rows {
		if r.Kind != usagecost.KindApp || !want[r.Name] {
			continue
		}
		c := r.ComputeCost + r.EgressCost
		mtd[r.Name] += c
		if r.Day != today {
			if byDay[r.Name] == nil {
				byDay[r.Name] = map[string]float64{}
			}
			byDay[r.Name][r.Day] += c
		}
	}
	out := make([]spendcap.AppLoad, 0, len(apps))
	for _, name := range apps {
		replicas := 0
		if svc, err := rt.apps.GetDesiredService(ctx, name); err == nil && !svc.Suspended {
			replicas = max(svc.Replicas, 1)
		}
		out = append(out, spendcap.AppLoad{Name: name, Replicas: replicas, MonthToDate: mtd[name], RecentDaily: recentDaily(byDay[name], view.now)})
	}
	return out
}

// handleAppSpendDryRun handles POST /api/v1/apps/{name}/budget/dry-run.
func (rt *Router) handleAppSpendDryRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: spend dry-run: load app", err, slog.String("name", name))
		return
	}
	view, err := rt.loadCostBudgetView(r.Context(), time.Now())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dryRunResource{
		Preview: spendcap.PreviewPause(rt.pauseLoads(r.Context(), view, []string{name}), view.now),
		Scope:   costbudget.ScopeApp, Target: name, Currency: view.currency, Note: dryRunNote,
	})
}

// handleCostBudgetDryRun handles POST /api/v1/costs/budgets/{id}/dry-run.
func (rt *Router) handleCostBudgetDryRun(w http.ResponseWriter, r *http.Request) {
	cs := rt.costBudgets()
	if cs == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	b, err := cs.GetCostBudget(r.Context(), r.PathValue("id"))
	if errors.Is(err, costbudget.ErrNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: spend dry-run: get budget", err)
		return
	}
	view, err := rt.loadCostBudgetView(r.Context(), time.Now())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dryRunResource{
		Preview: spendcap.PreviewPause(rt.pauseLoads(r.Context(), view, view.scopeApps(b)), view.now),
		Scope:   b.Scope, Target: b.ScopeValue, Currency: view.currency, Note: dryRunNote,
	})
}

// handleSpendOverride handles POST /api/v1/apps/{name}/budget/override: a
// root user lets deploys through for a short, audited window.
func (rt *Router) handleSpendOverride(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st, ok := rt.apps.(interface {
		SaveSpendOverride(ctx context.Context, o spendcap.Override, ev spendcap.Event) (spendcap.Override, error)
	})
	g := rt.spendGuard()
	if !ok || g == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	var req overrideRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: spend override: load app", err, slog.String("name", name))
		return
	}
	now := time.Now()
	actor := rt.eventActor(r)
	o, err := spendcap.NewOverride(name, req.Reason, actor, now, spendcap.OverrideTTL())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev := spendcap.Event{
		App: name, Period: o.Period, Kind: spendcap.EventOverride, At: now,
		Detail: fmt.Sprintf("%s allowed deploys until %s: %s", actor, o.ExpiresAt.Format(time.RFC3339), o.Reason),
	}
	if _, err := st.SaveSpendOverride(r.Context(), o, ev); err != nil {
		rt.internalError(w, "api: spend override", err, slog.String("name", name))
		return
	}
	status, err := g.Check(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: spend override: check", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, status)
}
