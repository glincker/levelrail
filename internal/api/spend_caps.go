package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/spendguard"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

const (
	defaultSpendCapHistory = 50
	maxSpendCapHistory     = 500
)

type spendCapResource struct {
	spendcap.Status
	// CostPriced is false while no prices are configured, so a cost ceiling
	// would measure zero.
	CostPriced bool `json:"cost_priced"`
	// Forecast is nil while usage tracking is unavailable.
	Forecast *spendForecast `json:"forecast,omitempty"`
	// Guard says whether new deploys are refused right now.
	Guard *spendguard.Status `json:"guard,omitempty"`
}

type spendCapRequest struct {
	MonthlyEgressBytes int64   `json:"monthly_egress_bytes"`
	MonthlyCostCeiling float64 `json:"monthly_cost_ceiling"`
	Thresholds         []int   `json:"thresholds"`
	Action             string  `json:"action"`
	ThrottleRPS        int     `json:"throttle_rps"`
	// GuardMode left empty keeps the cap's current mode, so older clients
	// cannot lift a deploy guard by omission.
	GuardMode string `json:"guard_mode"`
}

type spendCapHistoryResource struct {
	Events []spendcap.Event `json:"events"`
}

func (rt *Router) spendCaps() *spendcap.Controller {
	st, ok := rt.apps.(spendcap.Store)
	if !ok {
		return nil
	}
	return &spendcap.Controller{
		Store: st, Usage: rt, Logger: rt.logger,
		Nudge: func(context.Context) error { rt.nudgeReconciler(); return nil },
	}
}

// SpendCapUsage reports each app's month to date egress and cost from the
// same rows the cost showback uses.
func (rt *Router) SpendCapUsage(ctx context.Context, now time.Time) (map[string]spendcap.Usage, error) {
	st := rt.usageStore()
	if st == nil {
		return nil, errors.New("usage tracking is not available")
	}
	rt.usageEnsureFresh(ctx, now)
	monthStart := usagecost.StartOfMonth(now)
	rows, err := rt.usageRows(ctx, st, monthStart, monthStart.AddDate(0, 1, 0), now)
	if err != nil {
		return nil, err
	}
	out := map[string]spendcap.Usage{}
	for _, r := range rows {
		if r.Kind != usagecost.KindApp {
			continue
		}
		u := out[r.Name]
		u.EgressBytes += int64(r.EgressBytes)
		u.Cost += r.Total()
		out[r.Name] = u
	}
	return out, nil
}

// SpendCapTick is the scheduler entry point for the spend cap controller.
func (rt *Router) SpendCapTick(ctx context.Context) {
	c := rt.spendCaps()
	if c == nil {
		return
	}
	if err := c.Tick(ctx); err != nil {
		rt.logger.Warn("spend caps: reconcile failed", slog.String("error", err.Error()))
	}
}

// SpendCapBreaches feeds the budget_threshold alert rule.
func (rt *Router) SpendCapBreaches(ctx context.Context) ([]spendcap.Breach, error) {
	c := rt.spendCaps()
	if c == nil {
		return nil, nil
	}
	return c.Breaches(ctx)
}

func (rt *Router) spendCapResource(ctx context.Context, c *spendcap.Controller, app string) (spendCapResource, error) {
	s, err := c.Status(ctx, app)
	if err != nil {
		return spendCapResource{}, err
	}
	res := spendCapResource{Status: s}
	if st := rt.usageStore(); st != nil {
		if p, err := st.GetUsageCostSettings(ctx); err == nil {
			res.CostPriced = p.CPUCoreHour > 0 || p.MemGiBHour > 0 || p.StorageGiBMonth > 0 || p.EgressGiB > 0 || p.Mode == usagecost.ModeNode
		}
		res.Forecast = rt.appForecast(ctx, s.Cap, spendcap.Usage{EgressBytes: s.EgressBytes, Cost: s.Cost}, time.Now())
	}
	if g := rt.spendGuard(); g != nil {
		if gs, err := g.Check(ctx, app); err == nil {
			res.Guard = &gs
		}
	}
	return res, nil
}

func (rt *Router) handleGetSpendCap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	res, err := rt.spendCapResource(r.Context(), c, name)
	if errors.Is(err, spendcap.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no budget is set for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get spend cap", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handlePutSpendCap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	var req spendCapRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	current, curErr := c.Store.GetSpendCap(r.Context(), name)
	if curErr != nil && !errors.Is(curErr, spendcap.ErrNotFound) {
		rt.internalError(w, "api: set spend cap: load current", curErr, slog.String("name", name))
		return
	}
	prevMode := current.GuardMode
	if prevMode == "" {
		prevMode = spendcap.GuardAlert
	}
	if req.GuardMode == "" {
		req.GuardMode = prevMode
	}
	cp := spendcap.Cap{
		App: name, MonthlyEgressBytes: req.MonthlyEgressBytes, MonthlyCostCeiling: req.MonthlyCostCeiling,
		Thresholds: req.Thresholds, Action: req.Action, ThrottleRPS: req.ThrottleRPS, GuardMode: req.GuardMode,
	}.Normalize()
	if err := cp.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cp.GuardMode != prevMode && (cp.GuardMode == spendcap.GuardBlockDeploys || prevMode == spendcap.GuardBlockDeploys) && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, "changing the deploy guard needs a root token")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set spend cap: load app", err, slog.String("name", name))
		return
	}
	now := time.Now()
	ev := spendcap.Event{
		App: name, Period: spendcap.Period(now), Kind: spendcap.EventConfigured, At: now,
		Detail: fmt.Sprintf("%s set action %s, guard %s, egress %d bytes, cost ceiling %g", rt.eventActor(r), cp.Action, cp.GuardMode, cp.MonthlyEgressBytes, cp.MonthlyCostCeiling),
	}
	if err := c.Store.SaveSpendCap(r.Context(), cp, ev, now); err != nil {
		rt.internalError(w, "api: set spend cap", err, slog.String("name", name))
		return
	}
	if err := c.Tick(r.Context()); err != nil {
		rt.logger.Warn("spend caps: reconcile after save failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	res, err := rt.spendCapResource(r.Context(), c, name)
	if err != nil {
		rt.internalError(w, "api: set spend cap: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleDeleteSpendCap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	now := time.Now()
	ev := spendcap.Event{App: name, Period: spendcap.Period(now), Kind: spendcap.EventCleared, At: now, Detail: rt.eventActor(r) + " removed the budget"}
	err := c.Store.DeleteSpendCap(r.Context(), name, ev)
	if errors.Is(err, spendcap.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no budget is set for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: clear spend cap", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleSpendCapHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	limit := defaultSpendCapHistory
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxSpendCapHistory {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be between 1 and %d", maxSpendCapHistory))
			return
		}
		limit = n
	}
	events, err := c.Store.ListSpendCapEvents(r.Context(), name, limit)
	if err != nil {
		rt.internalError(w, "api: spend cap history", err, slog.String("name", name))
		return
	}
	if events == nil {
		events = []spendcap.Event{}
	}
	writeJSON(w, http.StatusOK, spendCapHistoryResource{Events: events})
}

type spendCapListResource struct {
	Caps []spendCapResource `json:"caps"`
}

// handleListSpendCaps handles GET /api/v1/costs/app-caps.
func (rt *Router) handleListSpendCaps(w http.ResponseWriter, r *http.Request) {
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	caps, err := c.Store.ListSpendCaps(r.Context())
	if err != nil {
		rt.internalError(w, "api: list spend caps", err)
		return
	}
	out := make([]spendCapResource, 0, len(caps))
	for _, cp := range caps {
		res, err := rt.spendCapResource(r.Context(), c, cp.App)
		if err != nil {
			rt.internalError(w, "api: list spend caps: status", err, slog.String("name", cp.App))
			return
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, spendCapListResource{Caps: out})
}
