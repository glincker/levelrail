package api

import (
	"net/http"
	"strconv"
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
	CodeLogin         bool `json:"code_login"`
	TrustedDeviceDays int  `json:"trusted_device_days"`
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
	out := loginOptionsResponse{CodeLogin: s.Admins || s.Others}
	if rt.newDeviceApproval {
		out.TrustedDeviceDays = max(1, int(rt.codeLogin.trustTTL.Hours()/24))
	}
	writeJSON(w, http.StatusOK, out)
}

// signInAttentionItems is at most one item per kind for the session user:
// a flood of requests collapses into a count with the newest requester's
// context, never one item each, and never a code.
func (rt *Router) signInAttentionItems(r *http.Request, now time.Time) []attention.Item {
	userID, ok := rt.currentSessionUserID(r)
	if !ok || rt.loginCodes == nil {
		return nil
	}
	ctx := r.Context()
	var items []attention.Item
	if codes, err := rt.loginCodes.ListLiveLoginCodesForUser(ctx, userID, now); err == nil && len(codes) > 0 {
		c := codes[0]
		ip, ua := safeSignInIP(c.RequesterIP), safeSignInText(c.UserAgent)
		items = append(items, feedItem(attention.Warning, attention.KindLoginCode, "account",
			strconv.Itoa(len(codes))+" sign-in code request(s), newest from "+ip+" ("+ua+")", signInParams(ip, ua, len(codes), c.CreatedAt, c.ExpiresAt)))
	}
	if approvals, err := rt.loginCodes.ListPendingLoginApprovalsForUser(ctx, userID, now); err == nil && len(approvals) > 0 {
		a := approvals[0]
		ip, ua := safeSignInIP(a.RequesterIP), safeSignInText(a.UserAgent)
		items = append(items, feedItem(attention.Warning, attention.KindLoginApproval, "account",
			"a password sign-in from a new browser at "+ip+" ("+ua+") waits for approval", signInParams(ip, ua, len(approvals), a.CreatedAt, a.ExpiresAt)))
	}
	return items
}

func signInParams(ip, ua string, count int, created, expires time.Time) map[string]string {
	return map[string]string{
		"ip": ip, "user_agent": ua, "count": strconv.Itoa(count),
		"at": created.UTC().Format(time.RFC3339), "expires_at": expires.UTC().Format(time.RFC3339),
	}
}
