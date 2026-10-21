package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

type certExpiryEmailResource struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// certEmailReadiness reports whether the signed-in user's address can receive cert alerts: a connected provider and a verified address.
func (rt *Router) certEmailReadiness(ctx context.Context, r *http.Request) (address, reason string) {
	c, ok := rt.emailSender.(interface{ Configured(context.Context) bool })
	if !ok || !c.Configured(ctx) {
		return "", "no_email_provider"
	}
	userID, found := rt.currentSessionUserID(r)
	if !found {
		return "", "no_session"
	}
	u, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil || u.Email == "" {
		return "", "no_address"
	}
	if !rt.authEngine.EmailVerified(ctx, u.Email) {
		return "", "address_not_verified"
	}
	return u.Email, ""
}

func (rt *Router) handleGetCertExpiryEmail(w http.ResponseWriter, r *http.Request) {
	if rt.alertDefaults == nil {
		writeError(w, http.StatusNotImplemented, "default alerts are not enabled")
		return
	}
	on, err := rt.alertDefaults.CertExpiryEmail(r.Context())
	if err != nil {
		rt.logger.Error("api: get cert expiry email failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	_, reason := rt.certEmailReadiness(r.Context(), r)
	writeJSON(w, http.StatusOK, certExpiryEmailResource{Enabled: on, Available: reason == "", Reason: reason})
}

func (rt *Router) handleSetCertExpiryEmail(w http.ResponseWriter, r *http.Request) {
	if rt.alertDefaults == nil {
		writeError(w, http.StatusNotImplemented, "default alerts are not enabled")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	address := ""
	if *req.Enabled {
		var reason string
		if address, reason = rt.certEmailReadiness(r.Context(), r); reason != "" {
			writeError(w, http.StatusConflict, "email is not available for cert alerts: "+reason)
			return
		}
	}
	if err := rt.alertDefaults.SetCertExpiryEmail(r.Context(), address, *req.Enabled); err != nil {
		rt.logger.Error("api: set cert expiry email failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, certExpiryEmailResource{Enabled: *req.Enabled, Available: true})
}
