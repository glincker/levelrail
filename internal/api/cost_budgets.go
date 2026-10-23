package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// Budget status levels.
const (
	costLevelOver     = "over"
	costLevelWarn     = "warn"
	costLevelForecast = "forecast"
)

// costBudgetStore is the persistence named budgets need; *store.DB satisfies it.
type costBudgetStore interface {
	ListCostBudgets(ctx context.Context) ([]costbudget.Budget, error)
	GetCostBudget(ctx context.Context, id string) (costbudget.Budget, error)
	SaveCostBudget(ctx context.Context, b costbudget.Budget, now time.Time) (costbudget.Budget, error)
	DeleteCostBudget(ctx context.Context, id string) error
	GetCostBudgetState(ctx context.Context, id string) (costbudget.State, bool, error)
	SaveCostBudgetState(ctx context.Context, st costbudget.State) error
}

func (rt *Router) costBudgets() costBudgetStore {
	if s, ok := rt.apps.(costBudgetStore); ok {
		return s
	}
	if s, ok := rt.policies.(costBudgetStore); ok {
		return s
	}
	return nil
}

// costBudgetStatus is a budget with its month to date position.
type costBudgetStatus struct {
	costbudget.Budget
	Currency         string         `json:"currency"`
	Period           string         `json:"period"`
	Spent            float64        `json:"spent"`
	Pct              float64        `json:"pct"`
	Forecast         usagecost.Band `json:"forecast"`
	ForecastPct      float64        `json:"forecast_pct"`
	Highest          int            `json:"highest"`
	Enforced         string         `json:"enforced"`
	Apps             []string       `json:"apps"`
	Level            string         `json:"level"`
	SuggestedMonthly float64        `json:"suggested_monthly"`
	ResetsAt         time.Time      `json:"resets_at"`
}

type costBudgetsResponse struct {
	Budgets  []costBudgetStatus `json:"budgets"`
	Currency string             `json:"currency"`
}

// costBudgetView is everything a budget evaluation reads, loaded once per pass.
type costBudgetView struct {
	rows     []usagecost.DailyCost
	tagsOf   map[string][]string
	apps     []usagecost.DailyUsage
	now      time.Time
	currency string
}

func (rt *Router) loadCostBudgetView(ctx context.Context, now time.Time) (costBudgetView, error) {
	st := rt.usageStore()
	if st == nil {
		return costBudgetView{}, errors.New("usage tracking is not available")
	}
	rt.usageEnsureFresh(ctx, now)
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		return costBudgetView{}, fmt.Errorf("cost budgets: settings: %w", err)
	}
	monthStart := usagecost.StartOfMonth(now)
	rows, err := rt.usageRows(ctx, st, monthStart, monthStart.AddDate(0, 1, 0), now)
	if err != nil {
		return costBudgetView{}, err
	}
	inv, err := rt.usageInventory(ctx)
	if err != nil {
		return costBudgetView{}, err
	}
	return costBudgetView{rows: rows, tagsOf: rt.tagNamesByResource(ctx), apps: inv, now: now, currency: settings.Currency}, nil
}

func (v costBudgetView) covers(b costbudget.Budget, kind, name, project string) bool {
	if kind != usagecost.KindApp && kind != usagecost.KindDatabase {
		return false
	}
	switch b.Scope {
	case costbudget.ScopeApp:
		return kind == usagecost.KindApp && name == b.ScopeValue
	case costbudget.ScopeProject:
		return project == b.ScopeValue
	case costbudget.ScopeTag:
		return slices.Contains(v.tagsOf[kind+":"+name], b.ScopeValue)
	}
	return false
}

// scopeApps lists the apps a budget covers. Databases count toward spend but
// have no ingress to throttle or pause, so they are never enforced.
func (v costBudgetView) scopeApps(b costbudget.Budget) []string {
	var out []string
	for _, u := range v.apps {
		if u.Kind == usagecost.KindApp && v.covers(b, u.Kind, u.Name, u.ProjectID) {
			out = append(out, u.Name)
		}
	}
	return out
}

func (v costBudgetView) spend(b costbudget.Budget) (spent float64, byDay map[string]float64) {
	today := usagecost.DayKey(usagecost.StartOfDay(v.now))
	byDay = map[string]float64{}
	for _, r := range v.rows {
		if !v.covers(b, r.Kind, r.Name, r.ProjectID) {
			continue
		}
		spent += r.Total()
		if r.Day != today {
			byDay[r.Day] += r.Total()
		}
	}
	return spent, byDay
}

func (v costBudgetView) status(b costbudget.Budget, st costbudget.State, hadState bool) costBudgetStatus {
	spent, byDay := v.spend(b)
	apps := v.scopeApps(b)
	res := costbudget.Step(b, st, hadState, v.now, spent, apps)
	var band usagecost.Band
	if elapsed := usagecost.ElapsedDays(v.now); elapsed >= 1 {
		band = usagecost.ForecastBand(spent, elapsed, usagecost.DaysInMonth(v.now), recentDaily(byDay, v.now))
	} else {
		band = usagecost.Band{Spent: spent, Mid: spent, Low: spent, High: spent, Confidence: usagecost.ConfidenceLow}
	}
	out := costBudgetStatus{
		Budget: b, Currency: v.currency, Period: res.State.Period, Spent: spent, Pct: res.State.Pct, Forecast: band,
		ForecastPct: costbudget.Pct(band.Mid, b.Monthly), Highest: res.State.Highest, Enforced: res.State.Enforced,
		Apps: apps, ResetsAt: spendcap.NextPeriodStart(v.now),
	}
	if out.Apps == nil {
		out.Apps = []string{}
	}
	switch {
	case out.Pct >= 100:
		out.Level = costLevelOver
	case out.Highest > 0:
		out.Level = costLevelWarn
	case band.Mid > b.Monthly:
		out.Level = costLevelForecast
	}
	if out.Level != "" {
		out.SuggestedMonthly = suggestMonthly(b.Monthly, spent, band)
	}
	return out
}

// suggestMonthly proposes a raised amount that covers the forecast range, or
// failing that a 20 percent step over spend, rounded up to a whole unit.
func suggestMonthly(current, spent float64, band usagecost.Band) float64 {
	want := math.Max(band.High, spent*1.2)
	if want <= current {
		want = current * 1.2
	}
	return math.Ceil(want)
}

// costBudgetStatuses returns every named budget's position.
func (rt *Router) costBudgetStatuses(ctx context.Context) ([]costBudgetStatus, string, error) {
	cs := rt.costBudgets()
	if cs == nil {
		return nil, "", nil
	}
	budgets, err := cs.ListCostBudgets(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("cost budgets: list: %w", err)
	}
	view, err := rt.loadCostBudgetView(ctx, time.Now())
	if err != nil {
		return nil, "", err
	}
	out := make([]costBudgetStatus, 0, len(budgets))
	for _, b := range budgets {
		st, had, err := cs.GetCostBudgetState(ctx, b.ID)
		if err != nil {
			return nil, "", fmt.Errorf("cost budgets: state: %w", err)
		}
		out = append(out, view.status(b, st, had))
	}
	return out, view.currency, nil
}

// CostBudgetTick is the scheduler entry point: it derives every budget's
// state from current spend, so a restart or a missed tick changes nothing.
func (rt *Router) CostBudgetTick(ctx context.Context) {
	cs := rt.costBudgets()
	if cs == nil {
		return
	}
	budgets, err := cs.ListCostBudgets(ctx)
	if err != nil || len(budgets) == 0 {
		if err != nil {
			rt.logger.Warn("cost budgets: list failed", slog.String("error", err.Error()))
		}
		return
	}
	now := time.Now()
	view, err := rt.loadCostBudgetView(ctx, now)
	if err != nil {
		rt.logger.Warn("cost budgets: usage unavailable, leaving enforcement as is", slog.String("error", err.Error()))
		return
	}
	nudge := false
	for _, b := range budgets {
		prev, had, err := cs.GetCostBudgetState(ctx, b.ID)
		if err != nil {
			rt.logger.Warn("cost budgets: load state", slog.String("budget", b.ID), slog.String("error", err.Error()))
			continue
		}
		spent, _ := view.spend(b)
		res := costbudget.Step(b, prev, had, now, spent, view.scopeApps(b))
		if !res.Changed {
			continue
		}
		if err := cs.SaveCostBudgetState(ctx, res.State); err != nil {
			rt.logger.Warn("cost budgets: save state", slog.String("budget", b.ID), slog.String("error", err.Error()))
			continue
		}
		for _, t := range res.Fired {
			rt.logger.Info("cost budgets: threshold reached", slog.String("budget", b.ID), slog.Int("threshold", t), slog.Float64("pct", res.State.Pct))
		}
		if prev.Enforced != res.State.Enforced || !slices.Equal(prev.EnforcedApps, res.State.EnforcedApps) {
			nudge = true
		}
	}
	if nudge {
		rt.nudgeReconciler()
	}
}

// CostBudgetBreaches feeds the budget_threshold alert rule with the highest
// threshold each budget fired this period.
func (rt *Router) CostBudgetBreaches(ctx context.Context) ([]costbudget.Breach, error) {
	cs := rt.costBudgets()
	if cs == nil {
		return nil, nil
	}
	budgets, err := cs.ListCostBudgets(ctx)
	if err != nil {
		return nil, fmt.Errorf("cost budgets: list: %w", err)
	}
	var currency string
	if st := rt.usageStore(); st != nil {
		if s, err := st.GetUsageCostSettings(ctx); err == nil {
			currency = s.Currency
		}
	}
	period := costbudget.Period(time.Now())
	var out []costbudget.Breach
	for _, b := range budgets {
		st, had, err := cs.GetCostBudgetState(ctx, b.ID)
		if err != nil {
			return nil, fmt.Errorf("cost budgets: state: %w", err)
		}
		if !had || st.Period != period {
			continue
		}
		if br, ok := costbudget.BreachFor(b, st, currency); ok {
			out = append(out, br)
		}
	}
	return out, nil
}

// handleListCostBudgets handles GET /api/v1/costs/budgets.
func (rt *Router) handleListCostBudgets(w http.ResponseWriter, r *http.Request) {
	if rt.costBudgets() == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	statuses, currency, err := rt.costBudgetStatuses(r.Context())
	if err != nil {
		rt.internalError(w, "api: list cost budgets", err)
		return
	}
	if statuses == nil {
		statuses = []costBudgetStatus{}
	}
	writeJSON(w, http.StatusOK, costBudgetsResponse{Budgets: statuses, Currency: currency})
}

// costBudgetInput is the create and update body; on update, nil leaves a
// field unchanged.
type costBudgetInput struct {
	Name        *string  `json:"name"`
	Scope       *string  `json:"scope"`
	ScopeValue  *string  `json:"scope_value"`
	Monthly     *float64 `json:"monthly"`
	Thresholds  *[]int   `json:"thresholds"`
	Action      *string  `json:"action"`
	ThrottleRPS *int     `json:"throttle_rps"`
	GuardMode   *string  `json:"guard_mode"`
}

func (in costBudgetInput) apply(b costbudget.Budget) costbudget.Budget {
	if in.Name != nil {
		b.Name = *in.Name
	}
	if in.Scope != nil {
		b.Scope = *in.Scope
	}
	if in.ScopeValue != nil {
		b.ScopeValue = *in.ScopeValue
	}
	if in.Monthly != nil {
		b.Monthly = *in.Monthly
	}
	if in.Thresholds != nil {
		b.Thresholds = *in.Thresholds
	}
	if in.Action != nil {
		b.Action = *in.Action
	}
	if in.ThrottleRPS != nil {
		b.ThrottleRPS = *in.ThrottleRPS
	}
	if in.GuardMode != nil {
		b.GuardMode = *in.GuardMode
	}
	return b
}

// resolveCostScope checks the scope target exists and maps a project name to
// its id, so the CLI and UI can both pass what the operator knows.
func (rt *Router) resolveCostScope(ctx context.Context, b costbudget.Budget) (costbudget.Budget, error) {
	switch b.Scope {
	case costbudget.ScopeApp:
		inv, err := rt.usageInventory(ctx)
		if err != nil {
			return b, err
		}
		for _, u := range inv {
			if u.Kind == usagecost.KindApp && u.Name == b.ScopeValue {
				return b, nil
			}
		}
		return b, fmt.Errorf("app %q does not exist", b.ScopeValue)
	case costbudget.ScopeProject:
		names := rt.usageScopeNames(ctx)
		if _, ok := names[usagecost.ScopeProject+":"+b.ScopeValue]; ok {
			return b, nil
		}
		for k, n := range names {
			if id, ok := strings.CutPrefix(k, usagecost.ScopeProject+":"); ok && strings.EqualFold(n, b.ScopeValue) {
				b.ScopeValue = id
				return b, nil
			}
		}
		return b, fmt.Errorf("project %q does not exist", b.ScopeValue)
	}
	return b, nil
}

func (rt *Router) saveCostBudget(w http.ResponseWriter, r *http.Request, base costbudget.Budget, status int) {
	cs := rt.costBudgets()
	var in costBudgetInput
	if err := decodeJSONBody(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	b := in.apply(base).Normalize()
	if err := b.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := rt.resolveCostScope(r.Context(), b)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := cs.SaveCostBudget(r.Context(), b, time.Now())
	if errors.Is(err, store.ErrCostBudgetNameTaken) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: save cost budget", err)
		return
	}
	rt.CostBudgetTick(r.Context())
	writeJSON(w, status, saved)
}

// handleCreateCostBudget handles POST /api/v1/costs/budgets.
func (rt *Router) handleCreateCostBudget(w http.ResponseWriter, r *http.Request) {
	if rt.costBudgets() == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	rt.saveCostBudget(w, r, costbudget.Budget{}, http.StatusCreated)
}

// handleUpdateCostBudget handles PUT /api/v1/costs/budgets/{id}.
func (rt *Router) handleUpdateCostBudget(w http.ResponseWriter, r *http.Request) {
	cs := rt.costBudgets()
	if cs == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	cur, err := cs.GetCostBudget(r.Context(), r.PathValue("id"))
	if errors.Is(err, costbudget.ErrNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get cost budget", err)
		return
	}
	rt.saveCostBudget(w, r, cur, http.StatusOK)
}

// handleDeleteCostBudget handles DELETE /api/v1/costs/budgets/{id}.
func (rt *Router) handleDeleteCostBudget(w http.ResponseWriter, r *http.Request) {
	cs := rt.costBudgets()
	if cs == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	err := cs.DeleteCostBudget(r.Context(), r.PathValue("id"))
	if errors.Is(err, costbudget.ErrNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete cost budget", err)
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusNoContent, nil)
}
