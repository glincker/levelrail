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

// NotificationDeliveryStore is the surface the delivery-history route and
// test-send recording need. *alerting.DB satisfies this structurally.
type NotificationDeliveryStore interface {
	RecordNotificationDelivery(ctx context.Context, d alerting.NotificationDelivery) error
	ListNotificationDeliveries(ctx context.Context, channelID string, limit int, before *time.Time) ([]alerting.NotificationDelivery, error)
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

// validNotifyKinds is shared by validateNotifyKind and every place that
// needs to list them (usage strings, error messages).
var validNotifyKinds = []alerting.NotifyKind{
	alerting.NotifyGeneric, alerting.NotifySlack, alerting.NotifyDiscord, alerting.NotifyTelegram,
	alerting.NotifyEmail, alerting.NotifyPushover, alerting.NotifyPagerDuty, alerting.NotifyTeams,
}

// validateNotifyKind is shared by channel creation and the test-send
// routes below.
func validateNotifyKind(kind string) (alerting.NotifyKind, error) {
	k := alerting.NotifyKind(kind)
	for _, valid := range validNotifyKinds {
		if k == valid {
			return k, nil
		}
	}
	return "", fmt.Errorf("kind must be one of %q", validNotifyKinds)
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
	sendErr := rt.notificationChannelTester.SendTest(ctx, channel.Kind, channel.NotifyURL)
	rt.recordNotificationDelivery(r.Context(), id, "test", sendErr)
	if sendErr != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("test notification failed: %s", sendErr.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// recordNotificationDelivery persists one delivery-history row for an
// existing-channel send (currently only the test-send route above; real
// deploy-outcome and alert-rule sends record their own history directly
// in internal/alerting, since those dispatch loops never pass through
// this package). No-op, logged not returned, when notificationDeliveries
// isn't configured or the write itself fails: a history-tracking failure
// must never affect the response this handler already sent.
func (rt *Router) recordNotificationDelivery(ctx context.Context, channelID, trigger string, sendErr error) {
	if rt.notificationDeliveries == nil {
		return
	}
	id, err := alerting.NewNotificationDeliveryID()
	if err != nil {
		rt.logger.Error("api: generate notification delivery id failed", slog.String("error", err.Error()))
		return
	}
	errMsg := ""
	if sendErr != nil {
		errMsg = sendErr.Error()
	}
	d := alerting.NotificationDelivery{ID: id, ChannelID: channelID, Trigger: trigger, Success: sendErr == nil, Error: errMsg}
	if err := rt.notificationDeliveries.RecordNotificationDelivery(ctx, d); err != nil {
		rt.logger.Error("api: record notification delivery failed", slog.String("channel_id", channelID), slog.String("error", err.Error()))
	}
}

// notificationDeliveryResource is the wire shape for one delivery-history
// row, mirroring alerting.NotificationDelivery field-for-field.
type notificationDeliveryResource struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Trigger   string `json:"trigger"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
}

func toNotificationDeliveryResource(d alerting.NotificationDelivery) notificationDeliveryResource {
	return notificationDeliveryResource{
		ID: d.ID, ChannelID: d.ChannelID, Trigger: d.Trigger,
		Success: d.Success, Error: d.Error, CreatedAt: d.CreatedAt,
	}
}

const (
	defaultNotificationDeliveryLimit = 50
	maxNotificationDeliveryLimit     = 200
)

// handleListNotificationDeliveries handles GET
// /api/v1/notification-channels/{id}/deliveries: every recorded send
// attempt for this channel, newest first, cursor-paginated by ?before
// (an RFC3339 timestamp) the same way GET /api/v1/audit-log already is.
// ?limit defaults to defaultNotificationDeliveryLimit, capped at
// maxNotificationDeliveryLimit.
func (rt *Router) handleListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	if rt.notificationChannels == nil || rt.notificationDeliveries == nil {
		writeError(w, http.StatusNotImplemented, "notification delivery history is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), id); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list notification deliveries: load channel failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	limit := defaultNotificationDeliveryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxNotificationDeliveryLimit {
		limit = maxNotificationDeliveryLimit
	}

	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC3339 timestamp")
			return
		}
		before = &t
	}

	deliveries, err := rt.notificationDeliveries.ListNotificationDeliveries(r.Context(), id, limit, before)
	if err != nil {
		rt.logger.Error("api: list notification deliveries failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]notificationDeliveryResource, len(deliveries))
	for i, d := range deliveries {
		out[i] = toNotificationDeliveryResource(d)
	}
	writeJSON(w, http.StatusOK, out)
}
