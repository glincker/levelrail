package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
)

// Same tiers as /api/v1/settings/vault: GET AbilityRead, writes and the live test AbilityRoot.
func (rt *Router) registerSecretProviderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings/secret-providers", rt.requireAbility(AbilityRead, rt.handleListSecretProviders))
	mux.HandleFunc("PUT /api/v1/settings/secret-providers/{name}", rt.requireAbility(AbilityRoot, rt.handlePutSecretProvider))
	mux.HandleFunc("DELETE /api/v1/settings/secret-providers/{name}", rt.requireAbility(AbilityRoot, rt.handleDeleteSecretProvider))
	mux.HandleFunc("POST /api/v1/settings/secret-providers/{name}/test", rt.requireAbility(AbilityRoot, rt.handleTestSecretProvider))
}

// secretProviderResource is one provider connection; the token is write-only and never returned.
type secretProviderResource struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	SelfHosted  bool   `json:"self_hosted"`
	HasToken    bool   `json:"has_token"`
	// ClientID is the non-secret Infisical machine identity ID; the client secret is never returned.
	ClientID   string     `json:"client_id,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	GraceUntil *time.Time `json:"rotation_grace_until,omitempty"`
}

type secretProviderListResponse struct {
	Providers []secretProviderResource `json:"providers"`
}

type secretProviderTestResponse struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

type putSecretProviderRequest struct {
	Type        string `json:"type"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	// Token is write-only; empty keeps the stored one.
	Token string `json:"token,omitempty"`
}

func (rt *Router) secretProviderStore() (secretprovider.Store, bool) {
	s, ok := rt.secrets.(secretprovider.Secrets)
	if !ok || s == nil {
		return secretprovider.Store{}, false
	}
	return secretprovider.Store{Secrets: s}, true
}

func toSecretProviderResource(cfg secretprovider.Config, hasToken bool) secretProviderResource {
	res := secretProviderResource{
		Name: cfg.Name, Type: cfg.Type, Project: cfg.Project, Environment: cfg.Environment,
		Path: cfg.Path, BaseURL: cfg.BaseURL, SelfHosted: cfg.BaseURL != "", HasToken: hasToken, UpdatedAt: cfg.UpdatedAt,
	}
	if !cfg.GraceUntil.IsZero() && time.Now().Before(cfg.GraceUntil) {
		g := cfg.GraceUntil
		res.GraceUntil = &g
	}
	return res
}

func (rt *Router) handleListSecretProviders(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.secretProviderStore()
	if !ok {
		writeJSON(w, http.StatusOK, secretProviderListResponse{Providers: []secretProviderResource{}})
		return
	}
	cfgs, err := st.List(r.Context())
	if err != nil {
		rt.internalError(w, "api: list secret providers failed", err)
		return
	}
	// The client ID needs a credential decrypt, so only root callers (the PUT tier) get it.
	abilities, aerr := rt.callerAbilities(r)
	canSeeID := aerr == nil && hasAbility(abilities, AbilityRoot)
	out := secretProviderListResponse{Providers: make([]secretProviderResource, 0, len(cfgs))}
	for _, c := range cfgs {
		res := toSecretProviderResource(c, st.HasToken(r.Context(), c.Name))
		if canSeeID {
			res.ClientID = identityClientID(r, st, c)
		}
		out.Providers = append(out.Providers, res)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handlePutSecretProvider(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.secretProviderStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "secret providers need a master key on this control plane")
		return
	}
	var req putSecretProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Validate decides which fields each type needs; project and environment are not required for onepassword.
	cfg := secretprovider.Config{
		Name: r.PathValue("name"), Type: req.Type, Project: req.Project,
		Environment: req.Environment, Path: req.Path, BaseURL: req.BaseURL,
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	grace := time.Duration(rotationGraceHours()) * time.Hour
	saved, err := st.Put(r.Context(), cfg, req.Token, grace, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.syncSecretConnections(r.Context())
	res := toSecretProviderResource(saved, st.HasToken(r.Context(), saved.Name))
	res.ClientID = identityClientID(r, st, saved)
	writeJSON(w, http.StatusOK, res)
}

// identityClientID returns only the client ID half of a stored machine identity credential; any failure yields "".
func identityClientID(r *http.Request, st secretprovider.Store, cfg secretprovider.Config) string {
	if cfg.Type != secretprovider.TypeInfisical {
		return ""
	}
	tok, _, err := st.Tokens(r.Context(), cfg, time.Now())
	if err != nil {
		return ""
	}
	id, _, ok := secretprovider.ParseIdentity(tok)
	if !ok {
		return ""
	}
	return id
}

func (rt *Router) handleDeleteSecretProvider(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.secretProviderStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "secret providers need a master key on this control plane")
		return
	}
	if err := st.Delete(r.Context(), r.PathValue("name")); err != nil {
		rt.internalError(w, "api: remove secret provider failed", err)
		return
	}
	rt.syncSecretConnections(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleTestSecretProvider(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.secretProviderStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "secret providers need a master key on this control plane")
		return
	}
	cfg, found, err := st.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: read secret provider failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "secret provider not found")
		return
	}
	token, _, err := st.Tokens(r.Context(), cfg, time.Now())
	if err != nil {
		writeJSON(w, http.StatusOK, secretProviderTestResponse{Reason: "stored token cannot be read; save the provider again with a token"})
		return
	}
	tun := secretprovider.TunablesFrom(rt.runtimeSettings)(r.Context())
	if err := secretprovider.Default().Test(r.Context(), cfg, token, tun); err != nil {
		writeJSON(w, http.StatusOK, secretProviderTestResponse{Reason: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, secretProviderTestResponse{OK: true})
}
