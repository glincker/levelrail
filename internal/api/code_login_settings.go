package api

import (
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
)

type codeLoginSettingsResource struct {
	Admins            bool `json:"admins"`
	Others            bool `json:"others"`
	Saved             bool `json:"saved"`
	NewDeviceApproval bool `json:"new_device_approval"`
}

type codeLoginSettingsRequest struct {
	Admins *bool `json:"admins"`
	Others *bool `json:"others"`
}

type loginOptionsResponse struct {
	CodeLogin bool `json:"code_login"`
}

// handleGetCodeLoginSettings handles GET /api/v1/settings/auth/code-login,
// the auth.code_login setting.
func (rt *Router) handleGetCodeLoginSettings(w http.ResponseWriter, r *http.Request) {
	_, saved, err := rt.loginCodes.GetCodeLoginSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get code login settings failed", err)
		return
	}
	s, err := rt.codeLoginSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get code login settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, codeLoginSettingsResource{Admins: s.Admins, Others: s.Others, Saved: saved, NewDeviceApproval: rt.newDeviceApproval})
}

// handlePutCodeLoginSettings handles PUT /api/v1/settings/auth/code-login.
// An omitted field keeps its current value.
func (rt *Router) handlePutCodeLoginSettings(w http.ResponseWriter, r *http.Request) {
	var req codeLoginSettingsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	s, err := rt.codeLoginSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: put code login settings failed", err)
		return
	}
	if req.Admins != nil {
		s.Admins = *req.Admins
	}
	if req.Others != nil {
		s.Others = *req.Others
	}
	s.UpdatedAt = time.Now()
	if err := rt.loginCodes.SaveCodeLoginSettings(r.Context(), s); err != nil {
		rt.internalError(w, "api: save code login settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, codeLoginSettingsResource{Admins: s.Admins, Others: s.Others, Saved: true, NewDeviceApproval: rt.newDeviceApproval})
}

// handleLoginOptions handles GET /api/v1/auth/login-options: public, so the
// sign-in page knows whether to offer codes at all. It says nothing about
// any one account.
func (rt *Router) handleLoginOptions(w http.ResponseWriter, r *http.Request) {
	s, err := rt.codeLoginSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: login options failed", err)
		return
	}
	writeJSON(w, http.StatusOK, loginOptionsResponse{CodeLogin: s.Admins || s.Others})
}

// signInAttentionItems lists the session user's own waiting codes and
// new-device approvals. Items carry requester context only, never a code.
func (rt *Router) signInAttentionItems(r *http.Request, now time.Time) []attention.Item {
	userID, ok := rt.currentSessionUserID(r)
	if !ok || rt.loginCodes == nil {
		return nil
	}
	ctx := r.Context()
	var items []attention.Item
	if codes, err := rt.loginCodes.ListLiveLoginCodesForUser(ctx, userID, now); err == nil {
		for _, c := range codes {
			it := feedItem(attention.Warning, attention.KindLoginCode, c.RequesterIP,
				"a sign-in code was requested from "+c.RequesterIP+" ("+c.UserAgent+")", signInParams(c.RequesterIP, c.UserAgent, c.CreatedAt, c.ExpiresAt))
			it.ID += ":" + c.ID
			items = append(items, it)
		}
	}
	if approvals, err := rt.loginCodes.ListPendingLoginApprovalsForUser(ctx, userID, now); err == nil {
		for _, a := range approvals {
			it := feedItem(attention.Warning, attention.KindLoginApproval, a.RequesterIP,
				"a password sign-in from a new browser at "+a.RequesterIP+" ("+a.UserAgent+") waits for approval", signInParams(a.RequesterIP, a.UserAgent, a.CreatedAt, a.ExpiresAt))
			it.ID += ":" + a.ID
			items = append(items, it)
		}
	}
	return items
}

func signInParams(ip, ua string, created, expires time.Time) map[string]string {
	return map[string]string{
		"ip": ip, "user_agent": ua,
		"at": created.UTC().Format(time.RFC3339), "expires_at": expires.UTC().Format(time.RFC3339),
	}
}
