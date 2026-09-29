package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// oidcRequestTokenTTL bounds how long a job run's runtime request token
// (not the OIDC token it mints, that keeps its own TTL from
// internal/oidc.Config.TTL) stays valid: generous relative to a job's
// own JobTimeout default, since teardown removes the entry the moment
// the job actually finishes anyway.
const oidcRequestTokenTTL = 6 * time.Hour

// oidcRequestEntry is what one job run's runtime token request resolves
// against: which audiences it may ask for, and the request template
// (Subject/Repo/Ref/PipelineID) OIDCIssuer needs, minus the audience
// itself, filled in per call.
type oidcRequestEntry struct {
	audiences map[string]bool
	req       OIDCTokenRequest
	expiresAt time.Time
}

// oidcRequestRegistry backs Engine.OIDCTokenRequestHandler: one entry
// per running job that opted into oidc, keyed by a random bearer token
// generated for that job run and never persisted. This is the "request
// a token at runtime for a given audience" pattern GitHub Actions'
// ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN uses, in place of pre-minting a
// token for every audience a job might need up front.
type oidcRequestRegistry struct {
	mu      sync.Mutex
	entries map[string]oidcRequestEntry
}

func newOIDCRequestRegistry() *oidcRequestRegistry {
	return &oidcRequestRegistry{entries: map[string]oidcRequestEntry{}}
}

func newOIDCRequestToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("pipeline: generate oidc request token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (r *oidcRequestRegistry) register(token string, audiences []string, req OIDCTokenRequest) {
	set := make(map[string]bool, len(audiences))
	for _, a := range audiences {
		if a != "" {
			set[a] = true
		}
	}
	r.mu.Lock()
	r.entries[token] = oidcRequestEntry{audiences: set, req: req, expiresAt: time.Now().Add(oidcRequestTokenTTL)}
	r.mu.Unlock()
}

func (r *oidcRequestRegistry) unregister(token string) {
	r.mu.Lock()
	delete(r.entries, token)
	r.mu.Unlock()
}

func (r *oidcRequestRegistry) lookup(token string) (oidcRequestEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[token]
	if !ok || time.Now().After(e.expiresAt) {
		return oidcRequestEntry{}, false
	}
	return e, true
}

type oidcTokenResponse struct {
	Token string `json:"token"`
}

// OIDCTokenRequestHandler serves the runtime token request endpoint a
// job script calls as PIPELINE_OIDC_REQUEST_URL: GET ?audience=X with
// "Authorization: Bearer <PIPELINE_OIDC_REQUEST_TOKEN>" mints a token
// for that one audience, only if the job's own `oidc` config allows it.
// Binding this to a container-reachable, never externally reachable
// address is the caller's job (see cmd/levelrail's wiring), not this
// package's: the bearer token is the actual access control either way.
func (e *Engine) OIDCTokenRequestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e.cfg.OIDCIssuer == nil {
			http.Error(w, "oidc is not configured on this control plane", http.StatusServiceUnavailable)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		entry, ok := e.oidcRequests.lookup(token)
		if !ok {
			http.Error(w, "unknown or expired request token", http.StatusUnauthorized)
			return
		}
		audience := r.URL.Query().Get("audience")
		if audience == "" || !entry.audiences[audience] {
			http.Error(w, "audience is not allowed for this job", http.StatusForbidden)
			return
		}
		req := entry.req
		req.Audience = audience
		tok, err := e.cfg.OIDCIssuer(r.Context(), req)
		if err != nil {
			http.Error(w, "mint token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oidcTokenResponse{Token: tok})
	})
}

// oidcAllowedAudiences returns every audience jd.OIDC allows, Audience
// plus Audiences deduplicated, or nil when jd.OIDC is nil.
func oidcAllowedAudiences(jd *Job) []string {
	if jd.OIDC == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range append([]string{jd.OIDC.Audience}, jd.OIDC.Audiences...) {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}
