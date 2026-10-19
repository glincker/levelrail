package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// ChannelQuietHours is the quiet-hours surface of alerting.DB.
type ChannelQuietHours interface {
	GetChannelQuietHours(ctx context.Context, channelID string) (*alerting.QuietHours, error)
	SetChannelQuietHours(ctx context.Context, q alerting.QuietHours) error
	DeleteChannelQuietHours(ctx context.Context, channelID string) error
}

type quietHoursResource struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
	Enabled  bool   `json:"enabled"`
}

func (rt *Router) quietHoursStore(w http.ResponseWriter, r *http.Request) (ChannelQuietHours, string, bool) {
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return nil, "", false
	}
	qh, ok := rt.notificationChannels.(ChannelQuietHours)
	if !ok {
		writeError(w, http.StatusNotImplemented, "quiet hours are not available on this control plane")
		return nil, "", false
	}
	id := r.PathValue("id")
	if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), id); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return nil, "", false
	} else if err != nil {
		rt.internalError(w, "quiet hours: load channel", err)
		return nil, "", false
	}
	return qh, id, true
}

// handleGetChannelQuietHours handles GET /api/v1/notification-channels/{id}/quiet-hours.
// A channel without quiet hours answers 200 with enabled false and empty times.
func (rt *Router) handleGetChannelQuietHours(w http.ResponseWriter, r *http.Request) {
	qh, id, ok := rt.quietHoursStore(w, r)
	if !ok {
		return
	}
	q, err := qh.GetChannelQuietHours(r.Context(), id)
	if err != nil {
		rt.internalError(w, "quiet hours: get", err)
		return
	}
	if q == nil {
		writeJSON(w, http.StatusOK, quietHoursResource{Timezone: "UTC"})
		return
	}
	writeJSON(w, http.StatusOK, quietHoursResource{Start: q.Start, End: q.End, Timezone: q.Timezone, Enabled: q.Enabled})
}

// handlePutChannelQuietHours handles PUT /api/v1/notification-channels/{id}/quiet-hours.
func (rt *Router) handlePutChannelQuietHours(w http.ResponseWriter, r *http.Request) {
	qh, id, ok := rt.quietHoursStore(w, r)
	if !ok {
		return
	}
	var req quietHoursResource
	if !decodeReq(w, r, &req) {
		return
	}
	q := alerting.QuietHours{ChannelID: id, Start: req.Start, End: req.End, Timezone: req.Timezone, Enabled: req.Enabled}
	if err := q.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := qh.SetChannelQuietHours(r.Context(), q); err != nil {
		rt.internalError(w, "quiet hours: set", err)
		return
	}
	writeJSON(w, http.StatusOK, quietHoursResource{Start: q.Start, End: q.End, Timezone: q.Timezone, Enabled: q.Enabled})
}

// handleDeleteChannelQuietHours handles DELETE /api/v1/notification-channels/{id}/quiet-hours.
func (rt *Router) handleDeleteChannelQuietHours(w http.ResponseWriter, r *http.Request) {
	qh, id, ok := rt.quietHoursStore(w, r)
	if !ok {
		return
	}
	if err := qh.DeleteChannelQuietHours(r.Context(), id); err != nil {
		rt.internalError(w, "quiet hours: delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
