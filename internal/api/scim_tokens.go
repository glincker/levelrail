package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxSCIMTokenName  = 64
	scimTokenShownLen = 10
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

func toSCIMTokenResource(t store.SCIMToken) scimTokenResource {
	fmtPtr := func(v *time.Time) *string {
		if v == nil {
			return nil
		}
		s := v.UTC().Format(time.RFC3339)
		return &s
	}
	return scimTokenResource{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339), LastUsedAt: fmtPtr(t.LastUsedAt), RevokedAt: fmtPtr(t.RevokedAt)}
}

func newSCIMSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate scim token: %w", err)
	}
	return scimTokenPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (rt *Router) mintSCIMToken(r *http.Request, name string) (store.SCIMToken, string, error) {
	secret, err := newSCIMSecret()
	if err != nil {
		return store.SCIMToken{}, "", err
	}
	id, err := randomOpaqueID("scimtok_")
	if err != nil {
		return store.SCIMToken{}, "", err
	}
	_, actorID, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = actorID
	}
	return store.SCIMToken{ID: id, Name: name, TokenHash: hashToken(secret), Prefix: secret[:scimTokenShownLen],
		CreatedBy: actor, CreatedAt: time.Now()}, secret, nil
}

func (rt *Router) scimAdminReady(w http.ResponseWriter) bool {
	if rt.scim == nil {
		writeError(w, http.StatusNotImplemented, errSSOUnavailable.Error())
		return false
	}
	return true
}

func (rt *Router) handleListSCIMTokens(w http.ResponseWriter, r *http.Request) {
	if !rt.scimAdminReady(w) {
		return
	}
	toks, err := rt.scim.ListSCIMTokens(r.Context())
	if err != nil {
		rt.internalError(w, "api: list scim tokens failed", err)
		return
	}
	out := make([]scimTokenResource, 0, len(toks))
	for _, t := range toks {
		out = append(out, toSCIMTokenResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.scimAdminReady(w) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxSCIMTokenName {
		writeError(w, http.StatusBadRequest, "name is required and at most 64 characters")
		return
	}
	tok, secret, err := rt.mintSCIMToken(r, name)
	if err != nil {
		rt.internalError(w, "api: create scim token failed", err)
		return
	}
	if err := rt.scim.CreateSCIMToken(r.Context(), tok); err != nil {
		rt.internalError(w, "api: create scim token failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, scimTokenCreated{scimTokenResource: toSCIMTokenResource(tok), Token: secret})
}

// handleRotateSCIMToken swaps a token for a new one with the same name; the old one stops working at once.
func (rt *Router) handleRotateSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.scimAdminReady(w) {
		return
	}
	old, err := rt.scim.GetSCIMToken(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSCIMTokenNotFound) || (err == nil && old.RevokedAt != nil) {
		writeError(w, http.StatusNotFound, "active scim token not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	next, secret, err := rt.mintSCIMToken(r, old.Name)
	if err != nil {
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	if err := rt.scim.RotateSCIMToken(r.Context(), old.ID, next); err != nil {
		if errors.Is(err, store.ErrSCIMTokenNotFound) {
			writeError(w, http.StatusNotFound, "active scim token not found")
			return
		}
		rt.internalError(w, "api: rotate scim token failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, scimTokenCreated{scimTokenResource: toSCIMTokenResource(next), Token: secret})
}

func (rt *Router) handleRevokeSCIMToken(w http.ResponseWriter, r *http.Request) {
	if !rt.scimAdminReady(w) {
		return
	}
	if err := rt.scim.RevokeSCIMToken(r.Context(), r.PathValue("id"), time.Now()); err != nil {
		if errors.Is(err, store.ErrSCIMTokenNotFound) {
			writeError(w, http.StatusNotFound, "active scim token not found")
			return
		}
		rt.internalError(w, "api: revoke scim token failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
