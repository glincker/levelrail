package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxSCIMTokenName  = 64
	scimTokenShownLen = 6
)

type scimTokenResource struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedBy  string  `json:"created_by"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
}

// scimTokenCreated carries the plaintext token, returned once and never stored.
type scimTokenCreated struct {
	scimTokenResource
	Token string `json:"token"`
}

func toSCIMTokenResource(t authengine.SCIMToken, m store.SCIMTokenMeta) scimTokenResource {
	fmtPtr := func(v *time.Time) *string {
		if v == nil {
			return nil
		}
		s := v.UTC().Format(time.RFC3339)
		return &s
	}
	return scimTokenResource{ID: t.ID, Name: t.Name, Prefix: m.Prefix, CreatedBy: m.CreatedBy,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339), LastUsedAt: fmtPtr(t.LastUsedAt), RevokedAt: fmtPtr(t.RevokedAt)}
}

// mintSCIMToken creates a library token and records who made it.
func (rt *Router) mintSCIMToken(r *http.Request, name string) (scimTokenCreated, error) {
	secret, tok, err := rt.authEngine.CreateSCIMToken(r.Context(), name)
	if err != nil {
		return scimTokenCreated{}, err
	}
	_, actorID, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = actorID
	}
	meta := store.SCIMTokenMeta{Prefix: secret[:scimTokenShownLen], CreatedBy: actor}
	if err := rt.sso.SaveSCIMTokenMeta(r.Context(), tok.ID, meta); err != nil {
		rt.logger.Warn("api: save scim token label failed", slog.String("token_id", tok.ID), slog.String("error", err.Error()))
	}
	return scimTokenCreated{scimTokenResource: toSCIMTokenResource(tok, meta), Token: secret}, nil
}

func (rt *Router) handleListSCIMTokens(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	toks, err := rt.authEngine.ListSCIMTokens(r.Context())
	if err != nil {
		rt.internalError(w, "api: list scim tokens failed", err)
		return
	}
	metas, err := rt.sso.SCIMTokenMetas(r.Context())
	if err != nil {
		rt.internalError(w, "api: list scim tokens failed", err)
		return
	}
	out := make([]scimTokenResource, 0, len(toks))
	for _, t := range toks {
		out = append(out, toSCIMTokenResource(t, metas[t.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxSCIMTokenName {
		writeError(w, http.StatusBadRequest, "name is required and at most 64 characters")
		return
	}
	created, err := rt.mintSCIMToken(r, name)
	if err != nil {
		rt.internalError(w, "api: create scim token failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleRotateSCIMToken mints a replacement with the same name, then revokes
// the old one. If the old one cannot be revoked the new one is revoked too,
// so a failed rotation never leaves two live tokens.
func (rt *Router) handleRotateSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	old, err := rt.authEngine.SCIMTokenByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, authengine.ErrSCIMTokenNotFound) || (err == nil && old.RevokedAt != nil) {
		writeError(w, http.StatusNotFound, "active scim token not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	next, err := rt.mintSCIMToken(r, old.Name)
	if err != nil {
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	if err := rt.authEngine.RevokeSCIMToken(r.Context(), old.ID); err != nil {
		if rerr := rt.authEngine.RevokeSCIMToken(context.WithoutCancel(r.Context()), next.ID); rerr != nil {
			rt.logger.Error("api: rotate scim token: revoking the replacement failed too", slog.String("token_id", next.ID), slog.String("error", rerr.Error()))
		}
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, next)
}

func (rt *Router) handleRevokeSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	tok, err := rt.authEngine.SCIMTokenByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, authengine.ErrSCIMTokenNotFound) || (err == nil && tok.RevokedAt != nil) {
		writeError(w, http.StatusNotFound, "active scim token not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: revoke scim token failed", err)
		return
	}
	if err := rt.authEngine.RevokeSCIMToken(r.Context(), tok.ID); err != nil {
		rt.internalError(w, "api: revoke scim token failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
