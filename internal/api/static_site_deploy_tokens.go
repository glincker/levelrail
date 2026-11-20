package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	siteTokenNameMax = 64
	msgSiteTokenDeny = "this token can only deploy its own Pages site"
)

// tokenSiteScopeStore is the site scope half of the token store.
type tokenSiteScopeStore interface {
	SetAPITokenSiteScope(ctx context.Context, s store.APITokenSiteScope) error
	GetAPITokenSiteScope(ctx context.Context, tokenID string) (*store.APITokenSiteScope, error)
	ListAPITokenIDsForSite(ctx context.Context, site string) ([]string, error)
}

func (rt *Router) tokenSiteScopes() tokenSiteScopeStore {
	if s, ok := rt.policies.(tokenSiteScopeStore); ok {
		return s
	}
	if s, ok := rt.apps.(tokenSiteScopeStore); ok {
		return s
	}
	if s, ok := rt.staticSites.(tokenSiteScopeStore); ok {
		return s
	}
	return nil
}

// siteTokenRoutes are the only routes a site token reaches: what the CLI and
// a curl pipeline need to upload an archive, start the deploy and read its state.
var siteTokenRoutes = []string{
	"GET /api/v1/apps/{name}",
	"POST /api/v1/apps/{name}/uploads",
	"POST /api/v1/apps/{name}/builds",
	"GET /api/v1/apps/{name}/deploys",
	"GET /api/v1/apps/{name}/deploy-attempts",
	"GET /api/v1/apps/{name}/uploads",
	"GET /api/v1/apps/{name}/deploys/{deployId}",
	"GET /api/v1/apps/{name}/deploys/{deployId}/logs",
	"GET /api/v1/static-sites/{name}",
}

// gateSiteToken confines a site scoped token to its own site and the routes
// above. Any lookup failure denies; a token without a site scope passes.
func (rt *Router) gateSiteToken(w http.ResponseWriter, r *http.Request, tokenID string) bool {
	ss := rt.tokenSiteScopes()
	if ss == nil {
		return true
	}
	sc, err := ss.GetAPITokenSiteScope(r.Context(), tokenID)
	if err != nil {
		rt.logger.Warn("api: site token scope lookup failed", slog.String("token_id", tokenID), slog.String("error", err.Error()))
		writeError(w, http.StatusForbidden, msgSiteTokenDeny)
		return false
	}
	if sc == nil {
		return true
	}
	if !slices.Contains(siteTokenRoutes, r.Pattern) || r.PathValue("name") != sc.SiteName {
		writeError(w, http.StatusForbidden, msgSiteTokenDeny)
		return false
	}
	return true
}

type siteDeployTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays int    `json:"expires_in_days"`
}

// siteDeployTokenResource never carries a secret; the plaintext is only in the create response.
type siteDeployTokenResource struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func (rt *Router) siteTokenDeps(w http.ResponseWriter, r *http.Request, ability string) (string, tokenSiteScopeStore, bool) {
	ss := rt.tokenSiteScopes()
	if ss == nil {
		writeError(w, http.StatusNotImplemented, "site deploy tokens are not configured on this control plane")
		return "", nil, false
	}
	site, _, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return "", nil, false
	}
	if !rt.authorizeRefs(w, r, ability, notFoundAs(http.StatusNotFound, "static site not found"), staticSiteRef(site.Name), appRef(site.Name)) {
		return "", nil, false
	}
	return site.Name, ss, true
}

// handleCreateSiteDeployToken handles POST /api/v1/static-sites/{name}/deploy-tokens.
func (rt *Router) handleCreateSiteDeployToken(w http.ResponseWriter, r *http.Request) {
	site, _, ok := rt.siteTokenDeps(w, r, AbilityDeploy)
	if !ok {
		return
	}
	var in siteDeployTokenRequest
	if err := decodeJSONBodyLimit(r, &in, 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.ExpiresInDays < 0 || len(in.Name) > siteTokenNameMax {
		writeError(w, http.StatusBadRequest, "expires_in_days must not be negative and name at most 64 characters")
		return
	}
	days := in.ExpiresInDays
	if days == 0 {
		days = rt.defaultTokenLifetimeDays(r.Context())
	}
	msg, err := rt.tokenLifetimeProblem(r.Context(), days)
	if err != nil {
		rt.internalError(w, "api: site deploy token: load security policy failed", err)
		return
	}
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	var expiresAt *time.Time
	if days > 0 {
		t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		expiresAt = &t
	}
	label := in.Name
	if label == "" {
		label = "ci"
	}
	req := createTokenRequest{Name: "pages:" + site + ":" + label, Abilities: []string{AbilityRead, AbilityDeploy}}
	ownerID, _ := rt.currentSessionUserID(r)
	scope := &store.APITokenScope{Site: &store.APITokenSiteScope{SiteName: site}}
	rt.libraryCreateToken(w, r, req, agentIdentity{}, ownerID, expiresAt, scope)
}

// handleListSiteDeployTokens handles GET /api/v1/static-sites/{name}/deploy-tokens.
func (rt *Router) handleListSiteDeployTokens(w http.ResponseWriter, r *http.Request) {
	site, ss, ok := rt.siteTokenDeps(w, r, AbilityRead)
	if !ok {
		return
	}
	ids, err := ss.ListAPITokenIDsForSite(r.Context(), site)
	if err != nil {
		rt.internalError(w, "api: site deploy tokens: list", err, slog.String("site", site))
		return
	}
	out := make([]siteDeployTokenResource, 0, len(ids))
	for _, id := range ids {
		t, err := rt.tokens.GetAPITokenByID(r.Context(), id)
		if errors.Is(err, store.ErrAPITokenNotFound) {
			continue
		}
		if err != nil {
			rt.internalError(w, "api: site deploy tokens: load", err, slog.String("token_id", id))
			return
		}
		out = append(out, siteDeployTokenResource{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeSiteDeployToken handles DELETE /api/v1/static-sites/{name}/deploy-tokens/{id}.
func (rt *Router) handleRevokeSiteDeployToken(w http.ResponseWriter, r *http.Request) {
	site, ss, ok := rt.siteTokenDeps(w, r, AbilityDeploy)
	if !ok {
		return
	}
	id := r.PathValue("id")
	sc, err := ss.GetAPITokenSiteScope(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: site deploy token: scope", err, slog.String("token_id", id))
		return
	}
	if sc == nil || sc.SiteName != site {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if err := rt.libraryRevokeToken(r.Context(), id); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
		rt.internalError(w, "api: site deploy token: revoke", err, slog.String("token_id", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
