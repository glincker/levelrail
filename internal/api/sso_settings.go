package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxSSOGroupMappings = 200
	maxSSOGroupNameLen  = 256
)

type ssoGroupMappingResource struct {
	Group    string `json:"group"`
	RoleID   string `json:"role_id"`
	Priority int    `json:"priority"`
}

type ssoGroupMappingsBody struct {
	Mappings []ssoGroupMappingResource `json:"mappings"`
}

type ssoSettingsResource struct {
	EnforceSSO       bool   `json:"enforce_sso"`
	SPCertificatePEM string `json:"sp_certificate"`
	BaseURL          string `json:"base_url"`
	BaseURLError     string `json:"base_url_error,omitempty"`
	MasterKeyMissing bool   `json:"master_key_missing"`
	SCIMBaseURL      string `json:"scim_base_url"`
	SCIMDefaultRole  string `json:"scim_default_role_id"`
}

// ssoSettingsRequest changes only the fields it carries.
type ssoSettingsRequest struct {
	EnforceSSO        *bool   `json:"enforce_sso"`
	SCIMDefaultRoleID *string `json:"scim_default_role_id"`
}

func (rt *Router) handleListSSOGroupMappings(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	ms, err := rt.sso.ListSSOGroupMappings(r.Context())
	if err != nil {
		rt.internalError(w, "api: list sso group mappings failed", err)
		return
	}
	out := ssoGroupMappingsBody{Mappings: make([]ssoGroupMappingResource, 0, len(ms))}
	for _, m := range ms {
		out.Mappings = append(out.Mappings, ssoGroupMappingResource(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutSSOGroupMappings replaces the whole mapping set in one step.
func (rt *Router) handlePutSSOGroupMappings(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	var body ssoGroupMappingsBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Mappings) > maxSSOGroupMappings {
		writeError(w, http.StatusBadRequest, "at most 200 group mappings")
		return
	}
	ms := make([]store.SSOGroupMapping, 0, len(body.Mappings))
	for _, m := range body.Mappings {
		g := strings.TrimSpace(m.Group)
		if g == "" || len(g) > maxSSOGroupNameLen || m.RoleID == "" {
			writeError(w, http.StatusBadRequest, "every mapping needs a group (at most 256 characters) and a role_id")
			return
		}
		ms = append(ms, store.SSOGroupMapping{Group: g, RoleID: m.RoleID, Priority: m.Priority})
	}
	err := rt.sso.ReplaceSSOGroupMappings(r.Context(), ms, time.Now())
	switch {
	case errors.Is(err, store.ErrSSOGroupDuplicate):
		writeError(w, http.StatusBadRequest, "a group is mapped more than once")
		return
	case errors.Is(err, store.ErrRoleNotFound):
		writeError(w, http.StatusBadRequest, "a role_id does not name a stored role")
		return
	case err != nil:
		rt.internalError(w, "api: replace sso group mappings failed", err)
		return
	}
	rt.handleListSSOGroupMappings(w, r)
}

func (rt *Router) ssoSettingsView(ctx context.Context) (ssoSettingsResource, error) {
	s, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		return ssoSettingsResource{}, err
	}
	res := ssoSettingsResource{EnforceSSO: s.EnforceSSO, SPCertificatePEM: s.SPCertificatePEM, MasterKeyMissing: rt.oauthSecrets == nil,
		SCIMBaseURL: rt.scimBase(ctx), SCIMDefaultRole: s.SCIMDefaultRoleID}
	if rt.oauthSecrets != nil {
		cert, _, err := rt.samlKeyPair(ctx)
		if err != nil {
			return ssoSettingsResource{}, err
		}
		res.SPCertificatePEM = cert
	}
	if base, err := rt.samlBaseURL(ctx); err != nil {
		res.BaseURLError = err.Error()
	} else {
		res.BaseURL = base
	}
	return res, nil
}

func (rt *Router) handleGetSSOSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	res, err := rt.ssoSettingsView(r.Context())
	if err != nil {
		rt.internalError(w, "api: get sso settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePutSSOSettings toggles SSO enforcement. Turning it on needs at least
// one enabled connection, so nobody but root is ever left with no way in.
func (rt *Router) handlePutSSOSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	var req ssoSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SCIMDefaultRoleID != nil && rt.scim != nil {
		err := rt.scim.SetSCIMDefaultRole(r.Context(), *req.SCIMDefaultRoleID, time.Now())
		if errors.Is(err, store.ErrRoleNotFound) {
			writeError(w, http.StatusBadRequest, "scim_default_role_id does not name a stored role")
			return
		}
		if err != nil {
			rt.internalError(w, "api: put sso settings failed", err)
			return
		}
	}
	if req.EnforceSSO == nil {
		rt.handleGetSSOSettings(w, r)
		return
	}
	if *req.EnforceSSO {
		conns, err := rt.sso.ListSAMLConnections(r.Context())
		if err != nil {
			rt.internalError(w, "api: put sso settings failed", err)
			return
		}
		if !slices.ContainsFunc(conns, func(c store.SAMLConnection) bool { return c.Enabled && samlConfigured(c) }) {
			writeError(w, http.StatusConflict, "enable a SAML connection before requiring single sign-on")
			return
		}
	}
	if err := rt.sso.SetSSOEnforce(r.Context(), *req.EnforceSSO, time.Now()); err != nil {
		rt.internalError(w, "api: put sso settings failed", err)
		return
	}
	rt.handleGetSSOSettings(w, r)
}

// ssoBlocksLocalSignIn reports whether a disabled account or SSO enforcement
// refuses a local sign-in for this user. Root users keep password sign-in as the break-glass path. A
// settings read failure refuses too: an unreadable policy must not open the door.
func (rt *Router) ssoBlocksLocalSignIn(ctx context.Context, userID string) bool {
	if rt.accountDisabled(ctx, userID) {
		return true
	}
	if rt.sso == nil {
		return false
	}
	s, err := rt.sso.GetSSOSettings(ctx)
	if err != nil {
		rt.logger.Error("api: read sso enforcement failed, refusing password sign-in", slog.String("user_id", userID), slog.String("error", err.Error()))
		return true
	}
	if !s.EnforceSSO {
		return false
	}
	user, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		return true
	}
	return !slices.Contains(user.Abilities, AbilityRoot)
}

const ssoRequiredMessage = "this account cannot sign in this way: use single sign-on from the sign-in page, or ask an administrator"
