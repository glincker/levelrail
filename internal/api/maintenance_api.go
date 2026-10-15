package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduledMaintenanceStore is the store surface for scheduled maintenance
// windows. *store.DB satisfies it.
type ScheduledMaintenanceStore interface {
	InsertMaintenanceWindow(ctx context.Context, w store.MaintenanceWindow) (store.MaintenanceWindow, error)
	ListMaintenanceWindows(ctx context.Context, since time.Time) ([]store.MaintenanceWindow, error)
	DeleteMaintenanceWindow(ctx context.Context, id string) error
}

// SetScheduledMaintenance enables the /api/v1/scheduled-maintenance routes.
func (rt *Router) SetScheduledMaintenance(s ScheduledMaintenanceStore) { rt.scheduledMaintenance = s }

// bannerLeadFloor is how early a window shows in the dashboard banner when
// its own announce lead time is shorter.
const bannerLeadFloor = 24 * time.Hour

type scheduledMaintenanceResource struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Message          string     `json:"message"`
	Scope            string     `json:"scope"`
	Target           string     `json:"target,omitempty"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	Freeze           bool       `json:"freeze"`
	AnnounceLeadMins int        `json:"announce_lead_minutes"`
	AnnouncedAt      *time.Time `json:"announced_at,omitempty"`
	State            string     `json:"state"`
	CreatedBy        string     `json:"created_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type scheduledMaintenanceRequest struct {
	Title            string    `json:"title"`
	Message          string    `json:"message"`
	Scope            string    `json:"scope"`
	Target           string    `json:"target"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	Freeze           bool      `json:"freeze"`
	AnnounceLeadMins int       `json:"announce_lead_minutes"`
}

func toScheduledMaintenanceResource(w store.MaintenanceWindow, now time.Time) scheduledMaintenanceResource {
	return scheduledMaintenanceResource{ID: w.ID, Title: w.Title, Message: w.Message, Scope: w.Scope, Target: w.Target,
		StartsAt: w.StartsAt, EndsAt: w.EndsAt, Freeze: w.Freeze, AnnounceLeadMins: w.AnnounceLeadMins,
		AnnouncedAt: w.AnnouncedAt, State: maintenance.StateAt(w, now), CreatedBy: w.CreatedBy, CreatedAt: w.CreatedAt}
}

func (rt *Router) scheduledMaintenanceOrNotImplemented(w http.ResponseWriter) (ScheduledMaintenanceStore, bool) {
	if rt.scheduledMaintenance == nil {
		writeError(w, http.StatusNotImplemented, "maintenance windows are not configured on this control plane")
		return nil, false
	}
	return rt.scheduledMaintenance, true
}

// handleListScheduledMaintenance handles GET /api/v1/scheduled-maintenance.
// With current=true it returns only what the dashboard banner shows: windows
// that have not ended and are active or inside their announce lead time.
func (rt *Router) handleListScheduledMaintenance(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.scheduledMaintenanceOrNotImplemented(w)
	if !ok {
		return
	}
	now := time.Now().UTC()
	since := now.Add(-7 * 24 * time.Hour)
	current := r.URL.Query().Get("current") == "true"
	if current {
		since = now
	}
	windows, err := st.ListMaintenanceWindows(r.Context(), since)
	if err != nil {
		rt.internalError(w, "list maintenance windows", err)
		return
	}
	out := make([]scheduledMaintenanceResource, 0, len(windows))
	for _, win := range windows {
		if current {
			lead := time.Duration(win.AnnounceLeadMins) * time.Minute
			if lead < bannerLeadFloor {
				lead = bannerLeadFloor
			}
			if now.Before(win.StartsAt.Add(-lead)) {
				continue
			}
		}
		out = append(out, toScheduledMaintenanceResource(win, now))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateScheduledMaintenance handles POST /api/v1/scheduled-maintenance.
func (rt *Router) handleCreateScheduledMaintenance(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.scheduledMaintenanceOrNotImplemented(w)
	if !ok {
		return
	}
	var req scheduledMaintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now().UTC()
	win, err := maintenance.Input{Title: req.Title, Message: req.Message, Scope: req.Scope, Target: req.Target,
		StartsAt: req.StartsAt, EndsAt: req.EndsAt, Freeze: req.Freeze, AnnounceLeadMins: req.AnnounceLeadMins}.Validate(now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	win.CreatedBy = actor
	created, err := st.InsertMaintenanceWindow(r.Context(), win)
	if err != nil {
		rt.internalError(w, "create maintenance window", err)
		return
	}
	rt.invalidateStatusPage()
	writeJSON(w, http.StatusCreated, toScheduledMaintenanceResource(created, now))
}

// handleDeleteScheduledMaintenance handles DELETE /api/v1/scheduled-maintenance/{id}.
func (rt *Router) handleDeleteScheduledMaintenance(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.scheduledMaintenanceOrNotImplemented(w)
	if !ok {
		return
	}
	err := st.DeleteMaintenanceWindow(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrMaintenanceWindowNotFound) {
		writeError(w, http.StatusNotFound, "maintenance window not found")
		return
	}
	if err != nil {
		rt.internalError(w, "delete maintenance window", err)
		return
	}
	rt.invalidateStatusPage()
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) invalidateStatusPage() {
	if rt.statusView != nil {
		rt.statusView.Invalidate()
	}
}
