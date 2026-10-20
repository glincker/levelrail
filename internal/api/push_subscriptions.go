package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// This file is browser push notification subscriptions (Settings ->
// Notification channels -> Browser push): the logged-in account's
// registered browsers, the delivery target the "webpush"
// notification-channel kind (internal/alerting, internal/webpush) sends
// to. Self-service like passkeys.go, so requireAuth not requireAbility.

// PushSubscriptions is the store surface the handlers below need.
// *store.DB satisfies this structurally.
type PushSubscriptions interface {
	SavePushSubscription(ctx context.Context, s store.PushSubscription) error
	ListPushSubscriptionsForUser(ctx context.Context, userID string) ([]store.PushSubscription, error)
	DeletePushSubscription(ctx context.Context, id, userID string) error
}

type pushSubscriptionResource struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

func toPushSubscriptionResource(s store.PushSubscription) pushSubscriptionResource {
	return pushSubscriptionResource{
		ID:        s.ID,
		UserAgent: s.UserAgent,
		CreatedAt: s.CreatedAt,
	}
}

// handleGetPushVAPIDPublicKey handles GET
// /api/v1/settings/push-subscriptions/vapid-public-key: the public half
// of this control plane's VAPID keypair, which the dashboard passes as
// PushManager.subscribe's applicationServerKey. Never the private half,
// which never leaves internal/secrets.
func (rt *Router) handleGetPushVAPIDPublicKey(w http.ResponseWriter, _ *http.Request) {
	if rt.pushVAPIDPublicKey == "" {
		writeError(w, http.StatusNotImplemented, "browser push is not configured on this control plane (no master key set)")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": rt.pushVAPIDPublicKey})
}

// handleListPushSubscriptions handles GET /api/v1/settings/push-subscriptions:
// every browser the caller's own account has registered.
func (rt *Router) handleListPushSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	subs, err := rt.pushSubscriptions.ListPushSubscriptionsForUser(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: list push subscriptions failed", err, slog.String("user_id", userID))
		return
	}
	out := make([]pushSubscriptionResource, 0, len(subs))
	for _, s := range subs {
		out = append(out, toPushSubscriptionResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// createPushSubscriptionRequest mirrors the PushSubscriptionJSON shape
// the browser's own PushSubscription.toJSON() produces: endpoint plus
// the two subscribe-time keys.
type createPushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	UserAgent string `json:"user_agent,omitempty"`
}

// handleCreatePushSubscription handles POST
// /api/v1/settings/push-subscriptions: registers (or re-registers, by
// endpoint) one browser subscription for the caller's own account.
func (rt *Router) handleCreatePushSubscription(w http.ResponseWriter, r *http.Request) {
	if rt.pushVAPIDPublicKey == "" {
		writeError(w, http.StatusNotImplemented, "browser push is not configured on this control plane (no master key set)")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createPushSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "endpoint, keys.p256dh, and keys.auth are required")
		return
	}

	id, err := store.NewPushSubscriptionID()
	if err != nil {
		rt.internalError(w, "api: create push subscription: generate id failed", err)
		return
	}

	sub := store.PushSubscription{
		ID: id, UserID: userID, Endpoint: req.Endpoint,
		P256dh: req.Keys.P256dh, Auth: req.Keys.Auth, UserAgent: req.UserAgent,
	}
	if err := rt.pushSubscriptions.SavePushSubscription(r.Context(), sub); err != nil {
		rt.internalError(w, "api: create push subscription failed", err, slog.String("user_id", userID))
		return
	}
	writeJSON(w, http.StatusCreated, toPushSubscriptionResource(sub))
}

// handleDeletePushSubscription handles DELETE
// /api/v1/settings/push-subscriptions/{id}: revokes one of the caller's
// own registered browsers (e.g. "forget this browser").
func (rt *Router) handleDeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("id")
	err := rt.pushSubscriptions.DeletePushSubscription(r.Context(), id, userID)
	if errors.Is(err, store.ErrPushSubscriptionNotFound) {
		writeError(w, http.StatusNotFound, "push subscription not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete push subscription failed", err, slog.String("id", id), slog.String("user_id", userID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
