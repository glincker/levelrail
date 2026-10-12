package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

func mobileAPIEnabled() bool {
	return !mobileAPIKilledByEnv() && experimental.Enabled(experimental.MobileAPI)
}

type mobileDeviceResource struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	TokenID    string     `json:"token_id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Expired    bool       `json:"expired"`
}

// callerOwnerID is the session user, or the owning user of a bearer token.
func (rt *Router) callerOwnerID(r *http.Request) string {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		return ""
	}
	if id, ok := rt.currentSessionUserID(r); ok {
		return id
	}
	if raw, ok := bearerToken(r); ok {
		if rec, err := rt.lookupBearerToken(r.Context(), raw); err == nil {
			return rec.OwnerUserID
		}
	}
	return ""
}

// handleListMobileDevices serves GET /api/v1/mobile/devices: root sees every device, others their own.
func (rt *Router) handleListMobileDevices(w http.ResponseWriter, r *http.Request) {
	ms, ok := rt.mobileStore(w)
	if !ok {
		return
	}
	owner := ""
	if !rt.callerIsAdmin(r) {
		if owner = rt.callerOwnerID(r); owner == "" {
			writeJSON(w, http.StatusOK, []mobileDeviceResource{})
			return
		}
	}
	devices, err := ms.ListMobileDevices(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: list mobile devices failed", err)
		return
	}
	recs, err := rt.authLib.tokens.ListTokens(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: list mobile devices: tokens failed", err)
		return
	}
	byID := make(map[string]int, len(recs))
	for i, t := range recs {
		byID[t.ID] = i
	}
	now := time.Now()
	out := make([]mobileDeviceResource, 0, len(devices))
	for _, d := range devices {
		i, found := byID[d.TokenID]
		if !found || recs[i].RevokedAt != nil {
			continue
		}
		t := recs[i]
		out = append(out, mobileDeviceResource{
			ID: d.ID, Name: d.Name, Platform: d.Platform, TokenID: d.TokenID, CreatedAt: d.CreatedAt,
			LastSeenAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, Expired: t.ExpiresAt != nil && now.After(*t.ExpiresAt),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeMobileDevice serves DELETE /api/v1/mobile/devices/{id}.
func (rt *Router) handleRevokeMobileDevice(w http.ResponseWriter, r *http.Request) {
	ms, ok := rt.mobileStore(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	d, err := ms.GetMobileDevice(r.Context(), id)
	if err == nil && !rt.callerIsAdmin(r) && d.OwnerUserID != rt.callerOwnerID(r) {
		err = store.ErrMobileDeviceNotFound
	}
	if errors.Is(err, store.ErrMobileDeviceNotFound) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: revoke mobile device: load failed", err, slog.String("device_id", id))
		return
	}
	if err := rt.libraryRevokeToken(r.Context(), d.TokenID); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
		rt.internalError(w, "api: revoke mobile device: token revoke failed", err, slog.String("device_id", id))
		return
	}
	if err := ms.RevokeMobileDevice(r.Context(), id, time.Now()); err != nil && !errors.Is(err, store.ErrMobileDeviceNotFound) {
		rt.internalError(w, "api: revoke mobile device failed", err, slog.String("device_id", id))
		return
	}
	rt.recordMobileAudit(r, store.AuditActionMobileRevoked, auditActorSession, rt.callerOwnerID(r), id, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
