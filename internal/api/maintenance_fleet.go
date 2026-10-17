package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envMaintenanceMaxDuration  = "APP_MAINTENANCE_MAX_DURATION"
	envMaintenanceAttentionAge = "APP_MAINTENANCE_ATTENTION_AFTER"

	defaultMaintenanceAttentionAge = 4 * time.Hour
)

// FleetMaintenanceStore lists every domain in maintenance with its owner.
type FleetMaintenanceStore interface {
	ListActiveDomainMaintenance(ctx context.Context, now time.Time) ([]store.DomainMaintenance, error)
	ListServiceDomains(ctx context.Context) ([]store.ServiceDomain, error)
}

type fleetMaintenanceEntry struct {
	App string `json:"app"`
	domainMaintenanceResource
	ElapsedSeconds int64 `json:"elapsed_seconds"`
}

type fleetMaintenanceResponse struct {
	Items []fleetMaintenanceEntry `json:"items"`
	// MaxDurationSeconds is the server default auto-off, 0 when none.
	MaxDurationSeconds int64 `json:"max_duration_seconds"`
	// AttentionAfterSeconds is how long maintenance may run before the
	// attention feed raises it.
	AttentionAfterSeconds int64 `json:"attention_after_seconds"`
}

func (rt *Router) fleetMaintenance(ctx context.Context, now time.Time) ([]fleetMaintenanceEntry, error) {
	fs, ok := rt.domainMaintenance.(FleetMaintenanceStore)
	if !ok {
		return []fleetMaintenanceEntry{}, nil
	}
	active, err := fs.ListActiveDomainMaintenance(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list active maintenance: %w", err)
	}
	owners, err := fs.ListServiceDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list domain owners: %w", err)
	}
	app := make(map[string]string, len(owners))
	for _, o := range owners {
		app[o.Domain] = o.ServiceName
	}
	out := make([]fleetMaintenanceEntry, 0, len(active))
	for _, m := range active {
		entry := fleetMaintenanceEntry{
			App: app[m.Domain], domainMaintenanceResource: toDomainMaintenanceResource(m),
			ElapsedSeconds: int64(maintenance.Elapsed(m.StartedAt, now).Seconds()),
		}
		rt.decorateMaintenance(ctx, &entry.domainMaintenanceResource, now)
		out = append(out, entry)
	}
	return out, nil
}

// handleListFleetMaintenance handles GET /api/v1/maintenance/domains:
// every domain in maintenance right now. AbilityRead.
func (rt *Router) handleListFleetMaintenance(w http.ResponseWriter, r *http.Request) {
	items, err := rt.fleetMaintenance(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "api: list fleet maintenance failed", err)
		return
	}
	writeJSON(w, http.StatusOK, fleetMaintenanceResponse{
		Items:                 items,
		MaxDurationSeconds:    int64(envDuration(envMaintenanceMaxDuration, 0).Seconds()),
		AttentionAfterSeconds: int64(envDuration(envMaintenanceAttentionAge, defaultMaintenanceAttentionAge).Seconds()),
	})
}

type maintenancePreviewRequest struct {
	Title             string `json:"title"`
	Message           string `json:"message"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

type maintenancePreviewResponse struct {
	HTML              string `json:"html"`
	Status            int    `json:"status"`
	RetryAfterSeconds string `json:"retry_after"`
}

// handlePreviewMaintenance handles POST /api/v1/maintenance/preview: the
// exact page and headers visitors get for these settings, rendered by the
// ingress itself so the preview cannot drift. AbilityRead, changes nothing.
func (rt *Router) handlePreviewMaintenance(w http.ResponseWriter, r *http.Request) {
	var req maintenancePreviewRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h := ingress.NewMaintenancePageHandler(ingress.MaintenancePage{
		Title: req.Title, Message: req.Message, RetryAfterSeconds: req.RetryAfterSeconds,
	})
	body := strings.NewReplacer(`\{`, "{", `\}`, "}").Replace(h.Body)
	retry := ""
	if v := h.Headers["Retry-After"]; len(v) > 0 {
		retry = v[0]
	}
	writeJSON(w, http.StatusOK, maintenancePreviewResponse{HTML: body, Status: h.StatusCode, RetryAfterSeconds: retry})
}

// maintenanceAttentionItems raises domains that have been in maintenance
// longer than APP_MAINTENANCE_ATTENTION_AFTER, so a site is not left down.
// Only domains of apps the caller can read are raised; unowned ones need root.
func (rt *Router) maintenanceAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	threshold := envDuration(envMaintenanceAttentionAge, defaultMaintenanceAttentionAge)
	items, err := rt.fleetMaintenance(r.Context(), now)
	if err != nil {
		rt.logger.Warn("api: attention feed: list maintenance failed", slog.String("error", err.Error()))
		return nil
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.logger.Warn("api: attention feed: resolve app visibility failed", slog.String("error", err.Error()))
		return nil
	}
	root := hasAbility(abilities, AbilityRoot)
	var out []attention.Item
	for _, e := range items {
		if e.App == "" && !root || e.App != "" && !canSee(e.App) {
			continue
		}
		if time.Duration(e.ElapsedSeconds)*time.Second <= threshold {
			continue
		}
		hours := e.ElapsedSeconds / 3600
		out = append(out, feedItem(attention.Warning, attention.KindMaintenanceLong, e.Domain,
			fmt.Sprintf("in maintenance for %dh, visitors still get the maintenance page", hours),
			map[string]string{"domain": e.Domain, "app": e.App, "hours": fmt.Sprint(hours)}))
	}
	return out
}
