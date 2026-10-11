package api

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

type usageNodeInfo struct {
	NodeID         string  `json:"node_id"`
	Name           string  `json:"name"`
	MonthlyPrice   float64 `json:"monthly_price"`
	VCPUs          float64 `json:"vcpus"`
	MemoryGiB      float64 `json:"memory_gib"`
	DetectedVCPUs  float64 `json:"detected_vcpus"`
	DetectedMemGiB float64 `json:"detected_memory_gib"`
	Priced         bool    `json:"priced"`
}

type usagePreset struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	MonthlyPrice float64 `json:"monthly_price"`
	VCPUs        float64 `json:"vcpus"`
	MemoryGiB    float64 `json:"memory_gib"`
	CPUCoreHour  float64 `json:"cpu_core_hour"`
	MemGiBHour   float64 `json:"mem_gib_hour"`
}

type usageSettingsResponse struct {
	Settings usagecost.Settings `json:"settings"`
	Nodes    []usageNodeInfo    `json:"nodes"`
	Presets  []usagePreset      `json:"presets"`
	Note     string             `json:"note"`
}

type usageSettingsRequest struct {
	Settings     *usagecost.Settings    `json:"settings"`
	Nodes        *[]usagecost.NodePrice `json:"nodes"`
	RepriceMonth bool                   `json:"reprice_month"`
}

const usageSettingsNote = "Prices are yours to set, not fetched from any provider. Each day is priced when it is snapshotted; changing prices does not rewrite history unless you ask for the month to date to be repriced."

// usagePresetTiers are illustrative typical VPS sizes, the same stance as
// the cost estimate's reference rates: a starting point to correct.
var usagePresetTiers = []usagePreset{
	{Key: "vps_small", Label: "Small VPS (2 vCPU, 4 GiB)", MonthlyPrice: 12, VCPUs: 2, MemoryGiB: 4},
	{Key: "vps_medium", Label: "Medium VPS (4 vCPU, 8 GiB)", MonthlyPrice: 24, VCPUs: 4, MemoryGiB: 8},
	{Key: "vps_large", Label: "Large VPS (8 vCPU, 16 GiB)", MonthlyPrice: 48, VCPUs: 8, MemoryGiB: 16},
}

func validateUsageSettings(s *usagecost.Settings) error {
	if s.Mode != usagecost.ModeUnit && s.Mode != usagecost.ModeNode {
		return errors.New("mode must be unit or node")
	}
	if s.Basis != usagecost.BasisAvg && s.Basis != usagecost.BasisP50 {
		return errors.New("basis must be avg or p50")
	}
	if !usageCurrencyRe.MatchString(s.Currency) {
		return errors.New("currency must be a 3 to 8 letter code")
	}
	for name, v := range map[string]float64{
		"cpu_core_hour": s.CPUCoreHour, "mem_gib_hour": s.MemGiBHour,
		"storage_gib_month": s.StorageGiBMonth, "egress_gib": s.EgressGiB,
	} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s must be zero or more", name)
		}
	}
	if s.IdleSpreadPct < 0 || s.IdleSpreadPct > 100 {
		return errors.New("idle_spread_pct must be between 0 and 100")
	}
	if s.CPUWeightPct <= 0 || s.CPUWeightPct > 100 {
		return errors.New("cpu_weight_pct must be between 1 and 100")
	}
	return nil
}

func validateUsageNodes(nodes []usagecost.NodePrice) error {
	seen := map[string]bool{}
	for _, n := range nodes {
		if n.NodeID == "" || seen[n.NodeID] {
			return errors.New("each node needs a unique node_id")
		}
		seen[n.NodeID] = true
		for _, v := range []float64{n.MonthlyPrice, n.VCPUs, n.MemoryGiB} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return errors.New("node price and capacity must be zero or more")
			}
		}
	}
	return nil
}

func (rt *Router) usageNodeInfos(r *http.Request, st usageCostStore, now time.Time) ([]usageNodeInfo, error) {
	ctx := r.Context()
	configured, err := st.ListUsageCostNodes(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{rt.advisorNodeOf(""): "This server"}
	if rt.nodes != nil {
		if nodes, err := rt.nodes.ListNodes(ctx); err == nil {
			for _, n := range nodes {
				names[n.ID] = n.Name
			}
		}
	}
	out := make([]usageNodeInfo, 0, len(names))
	for id, name := range names {
		v, m := rt.usageNodeCapacity(ctx, id, now)
		c := configured[id]
		out = append(out, usageNodeInfo{
			NodeID: id, Name: name, MonthlyPrice: c.MonthlyPrice, VCPUs: c.VCPUs, MemoryGiB: c.MemoryGiB,
			DetectedVCPUs: v, DetectedMemGiB: m, Priced: c.MonthlyPrice > 0,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, nil
}

// handleGetUsageSettings handles GET /api/v1/usage/settings.
func (rt *Router) handleGetUsageSettings(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	s, err := st.GetUsageCostSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: usage settings: load", err)
		return
	}
	nodes, err := rt.usageNodeInfos(r, st, time.Now())
	if err != nil {
		rt.internalError(w, "api: usage settings: nodes", err)
		return
	}
	presets := make([]usagePreset, len(usagePresetTiers))
	for i, p := range usagePresetTiers {
		p.CPUCoreHour, p.MemGiBHour = usagecost.UnitPricesFromNode(
			usagecost.NodePrice{MonthlyPrice: p.MonthlyPrice, VCPUs: p.VCPUs, MemoryGiB: p.MemoryGiB}, s.CPUWeightPct)
		presets[i] = p
	}
	writeJSON(w, http.StatusOK, usageSettingsResponse{Settings: s, Nodes: nodes, Presets: presets, Note: usageSettingsNote})
}

// handlePutUsageSettings handles PUT /api/v1/usage/settings.
func (rt *Router) handlePutUsageSettings(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	var req usageSettingsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := r.Context()
	now := time.Now()
	if req.Settings == nil && req.Nodes == nil && !req.RepriceMonth {
		writeError(w, http.StatusBadRequest, "send settings, nodes or reprice_month")
		return
	}
	if req.Settings != nil {
		if err := validateUsageSettings(req.Settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := st.SaveUsageCostSettings(ctx, *req.Settings, now); err != nil {
			rt.internalError(w, "api: usage settings: save", err)
			return
		}
	}
	if req.Nodes != nil {
		if err := validateUsageNodes(*req.Nodes); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := st.ReplaceUsageCostNodes(ctx, *req.Nodes, now); err != nil {
			rt.internalError(w, "api: usage settings: save nodes", err)
			return
		}
	}
	rt.usageCost.mu.Lock()
	rt.usageCost.liveAt = time.Time{}
	rt.usageCost.mu.Unlock()
	if req.RepriceMonth {
		start := usagecost.StartOfMonth(now)
		n, err := rt.usageReprice(ctx, st, start, usagecost.StartOfDay(now), now)
		if err != nil {
			rt.internalError(w, "api: usage settings: reprice", err)
			return
		}
		rt.logger.Info("usage cost: repriced month to date", slog.Int("days", n), slog.String("actor", rt.eventActor(r)))
	}
	rt.handleGetUsageSettings(w, r)
}

type usageBudgetRequest struct {
	Scope   string  `json:"scope"`
	ScopeID string  `json:"scope_id"`
	Monthly float64 `json:"monthly"`
	WarnPct float64 `json:"warn_pct"`
}

// usageScopeTeam resolves the team a budget scope belongs to, "" if the
// scope does not exist.
func (rt *Router) usageScopeTeam(r *http.Request, scope, id string) (string, error) {
	switch scope {
	case usagecost.ScopeTeam:
		if rt.organizations == nil {
			return "", errors.New("teams are not configured")
		}
		if _, err := rt.organizations.GetOrganization(r.Context(), id); err != nil {
			return "", nil
		}
		return id, nil
	case usagecost.ScopeProject:
		tdb := rt.teamDB()
		if tdb == nil {
			return store.DefaultTeamID, nil
		}
		team, err := tdb.TeamOfProject(r.Context(), id)
		if errors.Is(err, store.ErrProjectNotFound) {
			return "", nil
		}
		return team, err
	}
	return "", nil
}

// canManageUsageBudget: instance admins manage any budget, team scoped
// callers only those of teams where they are admin or owner.
func (rt *Router) canManageUsageBudget(r *http.Request, teamID string) (bool, error) {
	abilities, _, scope, err := rt.callerScope(r)
	if err != nil {
		return false, err
	}
	if scope.teams == nil {
		return hasAbility(abilities, AbilityRoot), nil
	}
	return store.TeamRoleAtLeast(scope.teams.roleOfTeam(teamID), store.TeamRoleAdmin), nil
}

func (rt *Router) canSeeUsageScope(r *http.Request, teamID string) (bool, error) {
	_, _, scope, err := rt.callerScope(r)
	if err != nil {
		return false, err
	}
	return scope.teams.allowsTeam(teamID), nil
}

type usageBudgetsResponse struct {
	Budgets  []usagecost.BudgetStatus `json:"budgets"`
	Currency string                   `json:"currency"`
}

// handleListUsageBudgets handles GET /api/v1/usage/budgets.
func (rt *Router) handleListUsageBudgets(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	ctx := r.Context()
	all, err := st.ListUsageBudgets(ctx)
	if err != nil {
		rt.internalError(w, "api: usage budgets: list", err)
		return
	}
	visible := all[:0:0]
	for _, b := range all {
		team, err := rt.usageScopeTeam(r, b.Scope, b.ScopeID)
		if err != nil {
			rt.internalError(w, "api: usage budgets: scope", err)
			return
		}
		ok, err := rt.canSeeUsageScope(r, team)
		if err != nil {
			rt.internalError(w, "api: usage budgets: visibility", err)
			return
		}
		if ok && team != "" {
			visible = append(visible, b)
		}
	}
	resp := usageBudgetsResponse{Budgets: []usagecost.BudgetStatus{}}
	settings, err := st.GetUsageCostSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: usage budgets: settings", err)
		return
	}
	resp.Currency = settings.Currency
	if len(visible) > 0 {
		if resp.Budgets, err = rt.budgetStatuses(ctx, st, visible, time.Now()); err != nil {
			rt.internalError(w, "api: usage budgets: status", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePutUsageBudget handles PUT /api/v1/usage/budgets: one budget per
// project or team, created or replaced.
func (rt *Router) handlePutUsageBudget(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	var req usageBudgetRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Scope != usagecost.ScopeProject && req.Scope != usagecost.ScopeTeam {
		writeError(w, http.StatusBadRequest, "scope must be project or team")
		return
	}
	if req.ScopeID == "" || req.Monthly <= 0 || math.IsNaN(req.Monthly) || math.IsInf(req.Monthly, 0) {
		writeError(w, http.StatusBadRequest, "scope_id and a positive monthly amount are required")
		return
	}
	if req.WarnPct == 0 {
		req.WarnPct = usagecost.DefaultWarnPct
	}
	if req.WarnPct < 1 || req.WarnPct > 100 {
		writeError(w, http.StatusBadRequest, "warn_pct must be between 1 and 100")
		return
	}
	team, err := rt.usageScopeTeam(r, req.Scope, req.ScopeID)
	if err != nil {
		rt.internalError(w, "api: usage budgets: scope", err)
		return
	}
	if team == "" {
		writeError(w, http.StatusNotFound, req.Scope+" not found")
		return
	}
	ok, err := rt.canManageUsageBudget(r, team)
	if err != nil {
		rt.internalError(w, "api: usage budgets: authorize", err)
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, msgTeamRoleTooLow)
		return
	}
	b, err := st.SaveUsageBudget(r.Context(), usagecost.Budget{Scope: req.Scope, ScopeID: req.ScopeID, Monthly: req.Monthly, WarnPct: req.WarnPct}, time.Now())
	if err != nil {
		rt.internalError(w, "api: usage budgets: save", err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// handleDeleteUsageBudget handles DELETE /api/v1/usage/budgets/{id}.
func (rt *Router) handleDeleteUsageBudget(w http.ResponseWriter, r *http.Request) {
	st := rt.usageStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, usageNotConfigured)
		return
	}
	b, err := st.GetUsageBudget(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrUsageBudgetNotFound) {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: usage budgets: load", err)
		return
	}
	team, err := rt.usageScopeTeam(r, b.Scope, b.ScopeID)
	if err != nil {
		rt.internalError(w, "api: usage budgets: scope", err)
		return
	}
	ok, err := rt.canManageUsageBudget(r, team)
	if err != nil {
		rt.internalError(w, "api: usage budgets: authorize", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "budget not found")
		return
	}
	if err := st.DeleteUsageBudget(r.Context(), b.ID); err != nil {
		rt.internalError(w, "api: usage budgets: delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
