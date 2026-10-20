package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	logBudgetResourcePrefix = "service:"
	logUsageCacheTTL        = 30 * time.Second
	maxLogBudgetMB          = 1 << 20
)

// LogBudgetStore is the per-app log budget and usage storage the routes need.
type LogBudgetStore interface {
	GetLogBudget(ctx context.Context, resourceID string) (int64, error)
	SetLogBudget(ctx context.Context, resourceID string, maxBytes int64) error
	DeleteLogBudget(ctx context.Context, resourceID string) error
	LogUsageAll(ctx context.Context, now time.Time, defaultMax int64) ([]telemetry.LogUsage, error)
	TrimLogBudget(ctx context.Context, resourceID string, defaultMax int64) (telemetry.LogBudgetTrim, bool, error)
}

// WithLogBudgetStore turns on the log budget and usage routes.
func WithLogBudgetStore(s LogBudgetStore) Option {
	return func(rt *Router) { rt.logBudgets = s }
}

// LogUsageApp is one app's stored log volume against its budget.
type LogUsageApp struct {
	App          string   `json:"app"`
	Bytes        int64    `json:"bytes"`
	Lines        int64    `json:"lines"`
	RateBytesMin float64  `json:"rate_bytes_per_min"`
	BudgetBytes  int64    `json:"budget_bytes"`
	BudgetCustom bool     `json:"budget_custom"`
	Percent      *float64 `json:"percent"`
	ForecastDays *float64 `json:"forecast_days"`
	TrimmedLines int64    `json:"trimmed_lines"`
	TrimmedBytes int64    `json:"trimmed_bytes"`
}

// LogUsageResponse is GET /api/v1/logs/usage.
type LogUsageResponse struct {
	DefaultBudgetBytes int64         `json:"default_budget_bytes"`
	Apps               []LogUsageApp `json:"apps"`
}

// LogBudgetResource is the per-app budget; MaxMB 0 means no override.
type LogBudgetResource struct {
	MaxMB            int64 `json:"max_mb"`
	DefaultMB        int64 `json:"default_mb"`
	EffectiveMaxByte int64 `json:"effective_max_bytes"`
}

type logUsageCache struct {
	mu      sync.Mutex
	at      time.Time
	entries []telemetry.LogUsage
}

func (c *logUsageCache) invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// logUsage serves the fleet aggregate from a 30s cache: the GROUP BY scans
// every log row, which is too much to repeat per page view.
func (rt *Router) logUsage(ctx context.Context) ([]telemetry.LogUsage, error) {
	c := &rt.logUsageCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < logUsageCacheTTL {
		return c.entries, nil
	}
	entries, err := rt.logBudgets.LogUsageAll(ctx, time.Now(), telemetry.DefaultLogBudgetBytes())
	if err != nil {
		return nil, err
	}
	c.entries, c.at = entries, time.Now()
	return entries, nil
}

func (rt *Router) logBudgetReady(w http.ResponseWriter) bool {
	if rt.logBudgets == nil {
		writeError(w, http.StatusNotImplemented, "log budgets are not configured on this control plane")
		return false
	}
	return true
}

// handleLogUsage handles GET /api/v1/logs/usage.
func (rt *Router) handleLogUsage(w http.ResponseWriter, r *http.Request) {
	if !rt.logBudgetReady(w) {
		return
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: log usage: visibility", err)
		return
	}
	entries, err := rt.logUsage(r.Context())
	if err != nil {
		rt.internalError(w, "api: log usage", err)
		return
	}
	out := LogUsageResponse{DefaultBudgetBytes: telemetry.DefaultLogBudgetBytes(), Apps: []LogUsageApp{}}
	for _, u := range entries {
		app, ok := strings.CutPrefix(u.ResourceID, logBudgetResourcePrefix)
		if !ok || !canSee(app) {
			continue
		}
		row := LogUsageApp{
			App: app, Bytes: u.Bytes, Lines: u.Lines, RateBytesMin: u.RatePerMin,
			BudgetBytes: u.BudgetBytes, BudgetCustom: u.BudgetCustom,
			ForecastDays: telemetry.ForecastDays(u.Bytes, u.BudgetBytes, u.RatePerMin),
			TrimmedLines: u.TrimmedLines, TrimmedBytes: u.TrimmedBytes,
		}
		if u.BudgetBytes > 0 {
			p := float64(u.Bytes) / float64(u.BudgetBytes) * 100
			row.Percent = &p
		}
		out.Apps = append(out.Apps, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func logBudgetResourceOf(maxBytes int64) LogBudgetResource {
	def := telemetry.DefaultLogBudgetBytes()
	res := LogBudgetResource{MaxMB: maxBytes >> 20, DefaultMB: def >> 20, EffectiveMaxByte: def}
	if maxBytes > 0 {
		res.EffectiveMaxByte = maxBytes
	}
	return res
}

// handleGetLogBudget handles GET /api/v1/apps/{name}/log-budget.
func (rt *Router) handleGetLogBudget(w http.ResponseWriter, r *http.Request) {
	if !rt.logBudgetReady(w) {
		return
	}
	name := r.PathValue("name")
	n, err := rt.logBudgets.GetLogBudget(r.Context(), logBudgetResourcePrefix+name)
	if err != nil {
		rt.internalError(w, "api: get log budget", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, logBudgetResourceOf(n))
}

// handleSetLogBudget handles PUT /api/v1/apps/{name}/log-budget.
func (rt *Router) handleSetLogBudget(w http.ResponseWriter, r *http.Request) {
	if !rt.logBudgetReady(w) {
		return
	}
	var req struct {
		MaxMB int64 `json:"max_mb"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.MaxMB < 0 || req.MaxMB > maxLogBudgetMB {
		writeError(w, http.StatusBadRequest, "max_mb must be between 0 and 1048576 (0 clears the budget)")
		return
	}
	name := r.PathValue("name")
	if err := rt.logBudgets.SetLogBudget(r.Context(), logBudgetResourcePrefix+name, req.MaxMB<<20); err != nil {
		rt.internalError(w, "api: set log budget", err, slog.String("name", name))
		return
	}
	rt.logUsageCache.invalidate()
	writeJSON(w, http.StatusOK, logBudgetResourceOf(req.MaxMB<<20))
}

// handleDeleteLogBudget handles DELETE /api/v1/apps/{name}/log-budget.
func (rt *Router) handleDeleteLogBudget(w http.ResponseWriter, r *http.Request) {
	if !rt.logBudgetReady(w) {
		return
	}
	name := r.PathValue("name")
	if err := rt.logBudgets.DeleteLogBudget(r.Context(), logBudgetResourcePrefix+name); err != nil {
		rt.internalError(w, "api: delete log budget", err, slog.String("name", name))
		return
	}
	rt.logUsageCache.invalidate()
	w.WriteHeader(http.StatusNoContent)
}

// LogTrimResult is POST /api/v1/apps/{name}/log-budget/trim.
type LogTrimResult struct {
	TrimmedLines int64 `json:"trimmed_lines"`
	TrimmedBytes int64 `json:"trimmed_bytes"`
	StillOver    bool  `json:"still_over"`
}

// handleTrimLogBudget handles POST /api/v1/apps/{name}/log-budget/trim.
func (rt *Router) handleTrimLogBudget(w http.ResponseWriter, r *http.Request) {
	if !rt.logBudgetReady(w) {
		return
	}
	name := r.PathValue("name")
	t, still, err := rt.logBudgets.TrimLogBudget(r.Context(), logBudgetResourcePrefix+name, telemetry.DefaultLogBudgetBytes())
	if err != nil {
		rt.internalError(w, "api: trim log budget", err, slog.String("name", name))
		return
	}
	rt.logUsageCache.invalidate()
	writeJSON(w, http.StatusOK, LogTrimResult{TrimmedLines: t.Lines, TrimmedBytes: t.Bytes, StillOver: still})
}
