package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

const mcpClientMaxNameLen = 64

type mcpClientResource struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes"`
	CreatedAt    string   `json:"created_at"`
}

type mcpClientRequest struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes"`
}

func toMCPClientResource(c store.MCPOAuthClient) mcpClientResource {
	return mcpClientResource{ID: c.ID, Name: c.Name, RedirectURIs: nonNilStrings(c.RedirectURIs), Scopes: nonNilStrings(c.Scopes),
		CreatedAt: time.Unix(c.CreatedAt, 0).UTC().Format(time.RFC3339)}
}

func (rt *Router) mcpClientStore(w http.ResponseWriter) (mcpOAuthStore, bool) {
	st := rt.mcpOAuth()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "OAuth for remote MCP is not available on this control plane")
	}
	return st, st != nil
}

func (rt *Router) handleCreateMCPClient(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.mcpClientStore(w)
	if !ok {
		return
	}
	var req mcpClientRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > mcpClientMaxNameLen {
		writeError(w, http.StatusBadRequest, "name is required and at most 64 characters")
		return
	}
	if len(req.RedirectURIs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one redirect URI is required")
		return
	}
	for _, u := range req.RedirectURIs {
		if !validRegisteredRedirect(u) {
			writeError(w, http.StatusBadRequest, "redirect URI "+u+" must be absolute, https (or http on 127.0.0.1 or [::1]) and have no fragment")
			return
		}
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{mcpScopeRead}
	}
	for _, s := range scopes {
		if mcpScopeAbility(s) == "" {
			writeError(w, http.StatusBadRequest, "scope "+s+" is not offered; use mcp:read or mcp:read:sensitive")
			return
		}
	}
	suffix, err := randomURLToken(12)
	if err != nil {
		rt.internalError(w, "api: create mcp client: id generation failed", err)
		return
	}
	userID, _ := rt.currentSessionUserID(r)
	c := store.MCPOAuthClient{
		ID: "mcpc_" + suffix, Name: name, RedirectURIs: slices.Compact(slices.Clone(req.RedirectURIs)),
		Scopes: slices.Compact(slices.Sorted(slices.Values(scopes))), CreatedBy: userID, CreatedAt: nowUnix(),
	}
	if err := st.CreateMCPOAuthClient(r.Context(), c); err != nil {
		rt.internalError(w, "api: create mcp client failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toMCPClientResource(c))
}

func (rt *Router) handleListMCPClients(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.mcpClientStore(w)
	if !ok {
		return
	}
	list, err := st.ListMCPOAuthClients(r.Context())
	if err != nil {
		rt.internalError(w, "api: list mcp clients failed", err)
		return
	}
	out := make([]mcpClientResource, 0, len(list))
	for _, c := range list {
		out = append(out, toMCPClientResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleDeleteMCPClient(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.mcpClientStore(w)
	if !ok {
		return
	}
	ids, err := st.DeleteMCPOAuthClient(r.Context(), r.PathValue("id"), nowUnix())
	if errors.Is(err, store.ErrMCPOAuthNotFound) {
		writeError(w, http.StatusNotFound, "no such MCP client")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete mcp client failed", err)
		return
	}
	for _, id := range ids {
		if err := rt.libraryRevokeToken(r.Context(), id); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			rt.logger.Warn("api: revoke mcp client token failed", "token_id", id)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
