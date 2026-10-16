package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainMaintenanceStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/maintenance need.
type DomainMaintenanceStore interface {
	GetDomainMaintenance(ctx context.Context, domain string) (bool, error)
	SetDomainMaintenance(ctx context.Context, domain string) error
	DeleteDomainMaintenance(ctx context.Context, domain string) error
	// ListDomainMaintenance backs GET /api/v1/domains's maintenance_enabled
	// flag in one bulk read.
	ListDomainMaintenance(ctx context.Context) ([]string, error)
}

// DomainMaintenanceSettingsStore is optional on DomainMaintenanceStore: it
// carries the page copy, Retry-After, bypass rules and the end time link.
type DomainMaintenanceSettingsStore interface {
	GetDomainMaintenanceSettings(ctx context.Context, domain string, now time.Time) (store.DomainMaintenance, bool, error)
	SaveDomainMaintenance(ctx context.Context, m store.DomainMaintenance) error
}

// domainMaintenanceResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/maintenance.
type domainMaintenanceResource struct {
	Domain            string   `json:"domain"`
	Enabled           bool     `json:"enabled"`
	Title             string   `json:"title,omitempty"`
	Message           string   `json:"message,omitempty"`
	RetryAfterSeconds int      `json:"retry_after_seconds,omitempty"`
	Until             string   `json:"until,omitempty"`
	WindowID          string   `json:"window_id,omitempty"`
	Bypass            []string `json:"bypass,omitempty"`
	BypassHealthCheck bool     `json:"bypass_health_check"`
	StartedAt         string   `json:"started_at,omitempty"`
	// AutoOffAt is the safety cut-off for open ended maintenance.
	AutoOffAt string `json:"auto_off_at,omitempty"`
}

// domainMaintenanceRequest is PUT's optional body; an empty body enables the
// default page.
type domainMaintenanceRequest struct {
	Title             string     `json:"title"`
	Message           string     `json:"message"`
	RetryAfterSeconds int        `json:"retry_after_seconds"`
	Until             *time.Time `json:"until"`
	Bypass            []string   `json:"bypass"`
	BypassHealthCheck *bool      `json:"bypass_health_check"`
	// MaxDurationSeconds switches open ended maintenance off after this long;
	// omitted takes the server default, 0 means no limit.
	MaxDurationSeconds *int `json:"max_duration_seconds"`
}

func toDomainMaintenanceResource(m store.DomainMaintenance) domainMaintenanceResource {
	res := domainMaintenanceResource{
		Domain: m.Domain, Enabled: true, Title: m.Title, Message: m.Message,
		RetryAfterSeconds: m.RetryAfterSeconds, WindowID: m.WindowID,
		Bypass: m.BypassCIDRs, BypassHealthCheck: m.BypassHealthCheck,
	}
	if !m.EndsAt.IsZero() {
		res.Until = m.EndsAt.UTC().Format(time.RFC3339)
	}
	if !m.StartedAt.IsZero() {
		res.StartedAt = m.StartedAt.UTC().Format(time.RFC3339)
	}
	if !m.AutoOffAt.IsZero() {
		res.AutoOffAt = m.AutoOffAt.UTC().Format(time.RFC3339)
	}
	return res
}

func (rt *Router) domainMaintenanceSettings() (DomainMaintenanceSettingsStore, bool) {
	s, ok := rt.domainMaintenance.(DomainMaintenanceSettingsStore)
	return s, ok
}

// handleGetDomainMaintenance handles GET
// /api/v1/apps/{name}/domains/{domain}/maintenance. AbilityRead.
func (rt *Router) handleGetDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if ss, ok := rt.domainMaintenanceSettings(); ok {
		m, on, err := ss.GetDomainMaintenanceSettings(r.Context(), domain, time.Now())
		if err != nil {
			rt.internalError(w, "api: get domain maintenance failed", err, slog.String("domain", domain))
			return
		}
		if !on {
			writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain})
			return
		}
		writeJSON(w, http.StatusOK, toDomainMaintenanceResource(m))
		return
	}
	enabled, err := rt.domainMaintenance.GetDomainMaintenance(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain maintenance failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: enabled})
}

// handleSetDomainMaintenance handles PUT
// /api/v1/apps/{name}/domains/{domain}/maintenance: enables maintenance mode
// with the optional page copy, end time and bypass rules. AbilityDeploy.
func (rt *Router) handleSetDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req domainMaintenanceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ss, ok := rt.domainMaintenanceSettings()
	if !ok {
		if err := rt.domainMaintenance.SetDomainMaintenance(r.Context(), domain); err != nil {
			rt.internalError(w, "api: set domain maintenance failed", err, slog.String("domain", domain))
			return
		}
		rt.nudgeReconciler()
		writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: true})
		return
	}
	now := time.Now().UTC()
	in := maintenance.DomainInput{
		Title: req.Title, Message: req.Message, RetryAfterSeconds: req.RetryAfterSeconds,
		Until: req.Until, Bypass: req.Bypass, BypassHealthCheck: req.BypassHealthCheck == nil || *req.BypassHealthCheck,
	}
	m, err := in.Validate(domain, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	prev, wasOn, err := ss.GetDomainMaintenanceSettings(r.Context(), domain, now)
	if err != nil {
		rt.internalError(w, "api: set domain maintenance: load previous", err, slog.String("domain", domain))
		return
	}
	var requested *time.Duration
	if req.MaxDurationSeconds != nil {
		d := time.Duration(*req.MaxDurationSeconds) * time.Second
		requested = &d
	}
	autoOff, err := maintenance.ResolveAutoOff(now, req.Until, requested, envDuration(envMaintenanceMaxDuration, 0))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m.StartedAt, m.AutoOffAt = maintenance.KeepStart(prev, wasOn, now), autoOff
	if req.Until != nil {
		if rt.scheduledMaintenance == nil {
			writeError(w, http.StatusNotImplemented, "an end time needs scheduled maintenance, which is not enabled on this server")
			return
		}
		win, err := rt.scheduledMaintenance.InsertMaintenanceWindow(r.Context(), maintenance.DomainWindow(m, r.PathValue("name"), now, *req.Until))
		if err != nil {
			rt.internalError(w, "api: set domain maintenance: create window", err, slog.String("domain", domain))
			return
		}
		m.WindowID, m.EndsAt = win.ID, win.EndsAt
	}
	if err := ss.SaveDomainMaintenance(r.Context(), m); err != nil {
		rt.dropMaintenanceWindow(r.Context(), m.WindowID)
		rt.internalError(w, "api: set domain maintenance failed", err, slog.String("domain", domain))
		return
	}
	if prev.WindowID != "" && prev.WindowID != m.WindowID {
		rt.dropMaintenanceWindow(r.Context(), prev.WindowID)
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, toDomainMaintenanceResource(m))
}

// NudgeReconciler asks for a prompt reconcile pass, for background jobs
// that change routing state outside a request.
func (rt *Router) NudgeReconciler() { rt.nudgeReconciler() }

// dropMaintenanceWindow removes a domain's linked window; best effort, since
// a leftover window only shows a stale status page entry.
func (rt *Router) dropMaintenanceWindow(ctx context.Context, id string) {
	if id == "" || rt.scheduledMaintenance == nil {
		return
	}
	if err := rt.scheduledMaintenance.DeleteMaintenanceWindow(ctx, id); err != nil && !errors.Is(err, store.ErrMaintenanceWindowNotFound) {
		rt.logger.Warn("api: remove domain maintenance window failed", slog.String("window_id", id), slog.String("error", err.Error()))
	}
}

// handleClearDomainMaintenance handles DELETE
// /api/v1/apps/{name}/domains/{domain}/maintenance: disables maintenance
// mode and cancels its linked window. AbilityDeploy. Idempotent.
func (rt *Router) handleClearDomainMaintenance(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var windowID string
	if ss, ok := rt.domainMaintenanceSettings(); ok {
		if m, on, err := ss.GetDomainMaintenanceSettings(r.Context(), domain, time.Now()); err == nil && on {
			windowID = m.WindowID
		}
	}
	if err := rt.domainMaintenance.DeleteDomainMaintenance(r.Context(), domain); err != nil {
		rt.internalError(w, "api: clear domain maintenance failed", err, slog.String("domain", domain))
		return
	}
	rt.dropMaintenanceWindow(r.Context(), windowID)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, domainMaintenanceResource{Domain: domain, Enabled: false})
}
