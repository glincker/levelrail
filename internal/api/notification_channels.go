package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// This file is global, connect-once notification channels (Settings ->
// Notification channels), attached by ID from deploy_notify_targets.go
// instead of retyping a URL per app. Mirrors backup_targets.go's shape.

// notificationChannelTestTimeout bounds the synchronous test-send
// handlers below, unlike the async dispatch paths elsewhere in this
// package which tolerate a slow target.
const notificationChannelTestTimeout = 10 * time.Second

// NotificationChannels is the store surface the channel handlers need.
// *alerting.DB satisfies this structurally.
type NotificationChannels interface {
	SaveNotificationChannel(ctx context.Context, c alerting.NotificationChannel) error
	GetNotificationChannel(ctx context.Context, id string) (*alerting.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context) ([]alerting.NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, id string) error
}

// NotificationChannelTester is the surface the test-send routes need: a
// real send through kind/notifyURL, not just a format check.
// *alerting.DeployDispatcher satisfies this structurally.
type NotificationChannelTester interface {
	SendTest(ctx context.Context, kind alerting.NotifyKind, notifyURL string) error
}

type notificationChannelResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toNotificationChannelResource(c alerting.NotificationChannel) notificationChannelResource {
	return notificationChannelResource{
		ID: c.ID, Name: c.Name, Kind: string(c.Kind), NotifyURL: c.NotifyURL,
		Enabled: c.Enabled, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// createNotificationChannelRequest is POST /api/v1/notification-channels'
// request body. Enabled defaults to true when omitted (a pointer so
// "false" and "not sent" are distinguishable).
type createNotificationChannelRequest struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

// validateNotifyKind is shared by channel creation and the test-send
// routes below, both accepting the same five kinds.
func validateNotifyKind(kind string) (alerting.NotifyKind, error) {
	k := alerting.NotifyKind(kind)
	switch k {
	case alerting.NotifyGeneric, alerting.NotifySlack, alerting.NotifyDiscord, alerting.NotifyTelegram, alerting.NotifyEmail, alerting.NotifyPushover:
		return k, nil
	default:
		return "", fmt.Errorf("kind must be one of %q, %q, %q, %q, %q, %q",
			alerting.NotifyGeneric, alerting.NotifySlack, alerting.NotifyDiscord, alerting.NotifyTelegram, alerting.NotifyEmail, alerting.NotifyPushover)
	}
}

func (req createNotificationChannelRequest) toChannel(id string) (alerting.NotificationChannel, error) {
	if req.Name == "" {
		return alerting.NotificationChannel{}, errors.New("name is required")
	}
	if req.NotifyURL == "" {
		return alerting.NotificationChannel{}, errors.New("notify_url is required")
	}
	kind, err := validateNotifyKind(req.Kind)
	if err != nil {
		return alerting.NotificationChannel{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return alerting.NotificationChannel{
		ID: id, Name: req.Name, Kind: kind, NotifyURL: req.NotifyURL, Enabled: enabled,
	}, nil
}

// handleListNotificationChannels handles GET /api/v1/notification-channels.
func (rt *Router) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}

	channels, err := rt.notificationChannels.ListNotificationChannels(r.Context())
	if err != nil {
		rt.logger.Error("api: list notification channels failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]notificationChannelResource, 0, len(channels))
	for _, c := range channels {
		out = append(out, toNotificationChannelResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateNotificationChannel handles POST /api/v1/notification-channels.
func (rt *Router) handleCreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}

	var req createNotificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := alerting.NewNotificationChannelID()
	if err != nil {
		rt.logger.Error("api: create notification channel: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	channel, err := req.toChannel(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := rt.notificationChannels.SaveNotificationChannel(r.Context(), channel); err != nil {
		rt.logger.Error("api: create notification channel failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toNotificationChannelResource(channel))
}

// handleDeleteNotificationChannel handles DELETE
// /api/v1/notification-channels/{id}. Unlike handleDeleteBackupTarget,
// this never 409s: the FK's ON DELETE SET NULL clears channel_id instead.
func (rt *Router) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	err := rt.notificationChannels.DeleteNotificationChannel(r.Context(), id)
	if errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete notification channel failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// testNotificationChannelRequest is POST
// /api/v1/notification-channels/test's body: kind+notify_url tested
// before the channel is ever saved.
type testNotificationChannelRequest struct {
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
}

// handleTestNotificationChannel handles
// POST /api/v1/notification-channels/test: fires one real test message
// via kind/notify_url without requiring the channel to exist yet.
func (rt *Router) handleTestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannelTester == nil {
		writeError(w, http.StatusNotImplemented, "notification channel testing is not configured on this control plane")
		return
	}

	var req testNotificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	kind, err := validateNotifyKind(req.Kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.NotifyURL == "" {
		writeError(w, http.StatusBadRequest, "notify_url is required")
		return
	}

	// This runs synchronously in the request handler, unlike the async
	// dispatch paths, so an unresponsive-but-connectable target can't
	// hang this goroutine indefinitely.
	ctx, cancel := context.WithTimeout(r.Context(), notificationChannelTestTimeout)
	defer cancel()
	if err := rt.notificationChannelTester.SendTest(ctx, kind, req.NotifyURL); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("test notification failed: %s", err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestExistingNotificationChannel handles POST
// /api/v1/notification-channels/{id}/test: the same real send, against
// an already-saved channel's own kind/notify_url.
func (rt *Router) handleTestExistingNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannels == nil || rt.notificationChannelTester == nil {
		writeError(w, http.StatusNotImplemented, "notification channel testing is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	channel, err := rt.notificationChannels.GetNotificationChannel(r.Context(), id)
	if errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: test notification channel: load channel failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationChannelTestTimeout)
	defer cancel()
	if err := rt.notificationChannelTester.SendTest(ctx, channel.Kind, channel.NotifyURL); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("test notification failed: %s", err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
