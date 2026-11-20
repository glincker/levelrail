package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envDeviceExpirySweep      = "APP_DEVICE_EXPIRY_SWEEP_INTERVAL" //nolint:gosec // env var name
	envAttentionWindow        = "APP_ATTENTION_RESOLVED_WINDOW"    //nolint:gosec // env var name
	envDeviceHistoryRetention = "APP_DEVICE_HISTORY_RETENTION"     //nolint:gosec // env var name

	defaultDeviceExpirySweep      = 30 * time.Second
	defaultAttentionWindow        = 24 * time.Hour
	defaultDeviceHistoryRetention = 30 * 24 * time.Hour

	deviceSweepBatch    = 200
	deviceActivityLimit = 200

	auditActorSystem         = "system"
	auditClientKindSystem    = "system"
	auditMethodEvent         = "EVENT"
	deviceLoginAuditActorID  = "device-login"
	deviceLoginAuditActor    = "Device login"
	deviceLoginItemKeyParts  = 3
	deviceLoginItemKeySep    = ":"
	deviceExpiredNoticeText  = "A CLI login request expired before anyone approved it. If you still need it, run the login again from the terminal."
	deviceActivityWindowFlag = "window"
)

// AttentionResolvedWindow is how long expired, denied and approved device
// logins stay visible, APP_ATTENTION_RESOLVED_WINDOW.
func AttentionResolvedWindow() time.Duration {
	return envDuration(envAttentionWindow, defaultAttentionWindow)
}

func deviceHistoryRetention() time.Duration {
	r := envDuration(envDeviceHistoryRetention, defaultDeviceHistoryRetention)
	if w := AttentionResolvedWindow(); r < w {
		return w
	}
	return r
}

// systemAuditEntry builds an audit row for an event with no request behind it.
func systemAuditEntry(action, path string, at time.Time) (store.AuditEntry, error) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return store.AuditEntry{}, err
	}
	return store.AuditEntry{
		ID: id, ActorType: auditActorSystem, ActorID: deviceLoginAuditActorID, ActorName: deviceLoginAuditActor,
		Ability: action, Method: auditMethodEvent, Path: path, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(at), ClientKind: auditClientKindSystem, Action: action,
	}, nil
}

// recordDeviceLoginAudit writes one audit entry for a session-driven device
// login event. The path carries the request id, never the user code.
func (rt *Router) recordDeviceLoginAudit(ctx context.Context, r *http.Request, action, userID, requestID string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: generate device login audit id failed", slog.String("request_id", requestID), slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSession, ActorID: userID, ActorName: rt.auditActorName(ctx, auditActorSession, userID, ""),
		Ability: AbilityWrite, Method: r.Method, Path: store.DeviceLoginAuditPath(requestID), StatusCode: http.StatusNoContent,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save device login audit entry failed", slog.String("request_id", requestID), slog.String("error", err.Error()))
	}
}

type deviceExpiryClaimer interface {
	ClaimDeviceLoginExpiry(ctx context.Context, requestID string, at time.Time, entry store.AuditEntry) (bool, error)
}

func (rt *Router) claimDeviceExpiry(ctx context.Context, requestID string, at time.Time) (bool, error) {
	c, ok := rt.auditLog.(deviceExpiryClaimer)
	if !ok {
		return false, errors.New("api: audit store cannot claim device login expiry")
	}
	entry, err := systemAuditEntry(store.AuditActionDeviceLoginExpired, store.DeviceLoginAuditPath(requestID), at)
	if err != nil {
		return false, err
	}
	return c.ClaimDeviceLoginExpiry(ctx, requestID, at, entry)
}

// expireDeviceRequestByDeviceCode records the expiry of a request the CLI
// polled after its TTL, sharing the sweeper's exactly-once claim.
func (rt *Router) expireDeviceRequestByDeviceCode(ctx context.Context, deviceCode string) {
	req, err := rt.authLib.device.DeviceRequestByDeviceCode(ctx, deviceCode, time.Now())
	if err != nil {
		return
	}
	claimed, err := rt.claimDeviceExpiry(ctx, req.ID, time.Now())
	if err != nil {
		rt.logger.Warn("api: record device login expiry failed", slog.String("request_id", req.ID), slog.String("error", err.Error()))
		return
	}
	if claimed {
		rt.notifyDeviceLoginExpired()
	}
}

// SweepDeviceLoginExpiry marks every pending request past its TTL as
// expired and audits each exactly once, returning how many it claimed.
func (rt *Router) SweepDeviceLoginExpiry(ctx context.Context, now time.Time) (int, error) {
	lapsed, err := rt.authLib.device.LapsedDevices(ctx, now, deviceSweepBatch)
	if err != nil {
		return 0, err
	}
	claimedCount := 0
	for _, d := range lapsed {
		claimed, err := rt.claimDeviceExpiry(ctx, d.ID, d.ExpiresAt)
		if err != nil {
			return claimedCount, err
		}
		if claimed {
			claimedCount++
			rt.logger.Info("api: device login expired", slog.String("request_id", d.ID))
		}
	}
	if claimedCount > 0 {
		rt.notifyDeviceLoginExpired()
	}
	return claimedCount, nil
}

// RunDeviceLoginExpirySweeper sweeps once at start, so expiries that passed
// while the server was down are audited, then on every interval tick.
func (rt *Router) RunDeviceLoginExpirySweeper(ctx context.Context) error {
	tick := func() {
		rt.accountSelf.sweep(time.Now())
		if _, err := rt.SweepDeviceLoginExpiry(ctx, time.Now()); err != nil && ctx.Err() == nil {
			rt.logger.Warn("api: device login expiry sweep failed", slog.String("error", err.Error()))
		}
		cutoff := time.Now().Add(-deviceHistoryRetention())
		if pruner, ok := rt.auditLog.(interface {
			PruneAttentionState(ctx context.Context, cutoff time.Time) error
		}); ok {
			if err := pruner.PruneAttentionState(ctx, cutoff); err != nil && ctx.Err() == nil {
				rt.logger.Warn("api: prune attention state failed", slog.String("error", err.Error()))
			}
		}
		if _, err := rt.authLib.device.PruneDevices(ctx, cutoff); err != nil && ctx.Err() == nil {
			rt.logger.Warn("api: prune device requests failed", slog.String("error", err.Error()))
		}
		if err := rt.SweepSignInExpiry(ctx, time.Now()); err != nil && ctx.Err() == nil {
			rt.logger.Warn("api: sign-in expiry sweep failed", slog.String("error", err.Error()))
		}
		if err := rt.SweepTokenHygiene(ctx, time.Now()); err != nil && ctx.Err() == nil {
			rt.logger.Warn("api: token hygiene sweep failed", slog.String("error", err.Error()))
		}
	}
	tick()
	ticker := time.NewTicker(envDuration(envDeviceExpirySweep, defaultDeviceExpirySweep))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			tick()
		}
	}
}

// notifyDeviceLoginExpired tells channels opted in to expiry notices that a
// login lapsed. Link only, and at most one per notice interval.
func (rt *Router) notifyDeviceLoginExpired() {
	if rt.deviceNotifier == nil || rt.notificationChannels == nil || !rt.deviceExpiryNotices.take(time.Now()) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), deviceNoticeTimeout)
		defer cancel()
		channels, err := rt.notificationChannels.ListNotificationChannels(ctx)
		if err != nil {
			rt.logger.Error("api: list notification channels for device login expiry notice failed", slog.String("error", err.Error()))
			return
		}
		text := deviceExpiredNoticeText
		if base := rt.dashboardURL(); base != "" {
			text += " " + base + deviceCLIPath
		}
		for _, c := range channels {
			if !c.Enabled || !c.NotifyDeviceLoginExpired {
				continue
			}
			if err := rt.deviceNotifier.SendNotice(ctx, c, text); err != nil {
				rt.logger.Error("api: send device login expiry notice failed",
					slog.String("channel_id", c.ID), slog.String("error", err.Error()))
			}
		}
	}()
}

type deviceActivityItem struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	ClientName  string    `json:"client_name"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	IPMismatch  bool      `json:"ip_mismatch"`
	// Dismissible is false while a request still waits.
	Dismissible bool `json:"dismissible"`
	// Dismissed is true when the calling session user already cleared it.
	Dismissed bool   `json:"dismissed"`
	ItemKey   string `json:"item_key"`
	AuditPath string `json:"audit_path"`
}

type deviceActivityResponse struct {
	WindowSeconds int                  `json:"window_seconds"`
	Items         []deviceActivityItem `json:"items"`
}

type dismissalLister interface {
	ListAttentionDismissals(ctx context.Context, userID string) (map[string]bool, error)
}

// handleDeviceActivity handles GET /api/v1/auth/device/activity: recent
// device logins in every state, with the caller's dismissals applied. It
// never carries a user code or device code.
func (rt *Router) handleDeviceActivity(w http.ResponseWriter, r *http.Request) {
	window := AttentionResolvedWindow()
	if raw := r.URL.Query().Get(deviceActivityWindowFlag); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "window must be a positive duration such as 24h")
			return
		}
		window = min(d, deviceHistoryRetention())
	}
	now := time.Now()
	reqs, err := rt.authLib.device.RecentDevices(r.Context(), now.Add(-window), now, deviceActivityLimit)
	if err != nil {
		rt.internalError(w, "api: device activity failed", err)
		return
	}
	var dismissed map[string]bool
	if userID, ok := rt.currentSessionUserID(r); ok {
		if l, ok := rt.auditLog.(dismissalLister); ok {
			if dismissed, err = l.ListAttentionDismissals(r.Context(), userID); err != nil {
				rt.internalError(w, "api: list attention dismissals failed", err)
				return
			}
		}
	}
	viewer := clientIP(r)
	out := deviceActivityResponse{WindowSeconds: int(window.Seconds()), Items: make([]deviceActivityItem, 0, len(reqs))}
	for _, d := range reqs {
		key := store.DeviceLoginItemKey(d.ID, d.State)
		out.Items = append(out.Items, deviceActivityItem{
			ID: d.ID, State: d.State, ClientName: d.ClientName, RequesterIP: d.RequesterIP, UserAgent: d.UserAgent,
			CreatedAt: d.CreatedAt, ExpiresAt: d.ExpiresAt, IPMismatch: d.RequesterIP != "" && d.RequesterIP != viewer,
			Dismissible: d.State != store.DeviceLoginStateWaiting, Dismissed: dismissed[key],
			ItemKey: key, AuditPath: store.DeviceLoginAuditPath(d.ID),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type attentionDismissRequest struct {
	ItemKey string `json:"item_key"`
	// ItemKeys dismisses several device logins in one call ("Dismiss all").
	ItemKeys []string `json:"item_keys"`
}

// maxDismissBatch bounds one batched dismissal to the activity list size.
const (
	maxDismissBatch     = deviceActivityLimit
	dismissBodyMaxBytes = 64 << 10
)

type dismissOutcome int

const (
	dismissDone dismissOutcome = iota
	dismissBadKey
	dismissMissing
	dismissNotDismissible
	dismissUnavailable
)

// handleAttentionDismiss handles POST /api/v1/attention/dismiss: a signed-in
// operator clears an informational item for themselves. The audit log is not
// edited, a dismissal adds its own entry.
func (rt *Router) handleAttentionDismiss(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req attentionDismissRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, dismissBodyMaxBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.ItemKeys) > 0 {
		rt.dismissDeviceLoginBatch(w, r, userID, req.ItemKeys)
		return
	}
	parts := strings.Split(req.ItemKey, deviceLoginItemKeySep)
	if len(parts) > 0 && parts[0] == attention.KindDataCopy {
		rt.dismissDataCopyItem(w, r, userID, req.ItemKey)
		return
	}
	switch out, err := rt.dismissDeviceLogin(r, userID, req.ItemKey); {
	case err != nil:
		rt.internalError(w, "api: dismiss attention item failed", err)
	case out == dismissBadKey:
		writeError(w, http.StatusBadRequest, "item_key is not a dismissible item")
	case out == dismissMissing:
		writeError(w, http.StatusNotFound, "no such item")
	case out == dismissNotDismissible:
		writeError(w, http.StatusConflict, "item is not in a dismissible state")
	case out == dismissUnavailable:
		writeError(w, http.StatusNotImplemented, "dismissal is not available")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// dismissDeviceLoginBatch clears many logins at once. A malformed key rejects
// the whole call before any write; keys that went stale (pruned, or no longer
// in that state) are skipped, since the goal is only that they stop showing.
func (rt *Router) dismissDeviceLoginBatch(w http.ResponseWriter, r *http.Request, userID string, keys []string) {
	if len(keys) > maxDismissBatch {
		writeError(w, http.StatusBadRequest, "too many item_keys")
		return
	}
	for _, k := range keys {
		if p := strings.Split(k, deviceLoginItemKeySep); len(p) != deviceLoginItemKeyParts || p[0] != store.AuditActionFamilyDeviceLogin {
			writeError(w, http.StatusBadRequest, "item_key is not a dismissible item")
			return
		}
	}
	for _, k := range keys {
		out, err := rt.dismissDeviceLogin(r, userID, k)
		if err != nil {
			rt.internalError(w, "api: dismiss attention items failed", err)
			return
		}
		if out == dismissUnavailable {
			writeError(w, http.StatusNotImplemented, "dismissal is not available")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// dismissDeviceLogin records one dismissal. The returned error is only for
// storage failures, every client-caused refusal is a dismissOutcome.
func (rt *Router) dismissDeviceLogin(r *http.Request, userID, itemKey string) (dismissOutcome, error) {
	parts := strings.Split(itemKey, deviceLoginItemKeySep)
	if len(parts) != deviceLoginItemKeyParts || parts[0] != store.AuditActionFamilyDeviceLogin {
		return dismissBadKey, nil
	}
	requestID, state := parts[1], parts[2]
	cur, err := rt.authLib.device.DeviceRequestByID(r.Context(), requestID, time.Now())
	if err != nil {
		return dismissMissing, nil
	}
	if cur.State == store.DeviceLoginStateWaiting || cur.State != state {
		return dismissNotDismissible, nil
	}
	d, ok := rt.auditLog.(interface {
		DismissAttentionItem(ctx context.Context, userID, itemKey string, at time.Time, entry *store.AuditEntry) (bool, error)
	})
	if !ok {
		return dismissUnavailable, nil
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return dismissDone, fmt.Errorf("audit id: %w", err)
	}
	now := time.Now()
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSession, ActorID: userID, ActorName: rt.auditActorName(r.Context(), auditActorSession, userID, ""),
		Ability: AbilityWrite, Method: r.Method, Path: store.DeviceLoginAuditPath(requestID), StatusCode: http.StatusNoContent,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(now),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: store.AuditActionDeviceLoginDismissed,
	}
	if _, err := d.DismissAttentionItem(r.Context(), userID, itemKey, now, &entry); err != nil {
		return dismissDone, err
	}
	return dismissDone, nil
}
