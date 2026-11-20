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
	siteTokenNameMax  = 64
	msgSiteTokenDeny  = "this token can only deploy its own Pages site"
	msgSiteTokenBuild = "this token can only start a static upload deploy of its own Pages site"
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

type siteTokenCtxKey struct{}

// siteScopedRequest reports whether the gate marked this request as made by a site token.
func siteScopedRequest(r *http.Request) bool {
	v, _ := r.Context().Value(siteTokenCtxKey{}).(bool)
	return v
}

// gateSiteToken confines a site scoped token to its own site and the routes
// above. Any lookup failure denies; a token without a site scope passes.
func (rt *Router) gateSiteToken(w http.ResponseWriter, r *http.Request, tokenID string) (*http.Request, bool) {
	ss := rt.tokenSiteScopes()
	if ss == nil {
		return r, true
	}
	sc, err := ss.GetAPITokenSiteScope(r.Context(), tokenID)
	if err != nil {
		rt.logger.Warn("api: site token scope lookup failed", slog.String("token_id", tokenID), slog.String("error", err.Error()))
		rt.denySiteToken(w, r, tokenID)
		return r, false
	}
	if sc == nil {
		return r, true
	}
	if !slices.Contains(siteTokenRoutes, r.Pattern) || r.PathValue("name") != sc.SiteName || !rt.siteScopeTargetsPages(r.Context(), sc.SiteName) {
		rt.denySiteToken(w, r, tokenID)
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), siteTokenCtxKey{}, true)), true
}

// denySiteToken audits the refusal before answering, so a probing token leaves a trace.
func (rt *Router) denySiteToken(w http.ResponseWriter, r *http.Request, tokenID string) {
	rt.recordAudit(r.Context(), r, AbilityDeploy, "token", tokenID, "", http.StatusForbidden)
	writeError(w, http.StatusForbidden, msgSiteTokenDeny)
}

// siteScopeTargetsPages holds a scope to a live Pages site, so a reused app name never inherits it.
func (rt *Router) siteScopeTargetsPages(ctx context.Context, name string) bool {
	app, err := rt.apps.GetDesiredService(ctx, name)
	return err == nil && app != nil && rt.isPagesSiteApp(ctx, name, app.Image)
}

// siteTokenBuildProblem limits a site token to a static upload deploy of its own Pages site.
func (rt *Router) siteTokenBuildProblem(ctx context.Context, name, image string, req triggerBuildRequest) string {
	b := req.Build
	t := b.Type
	if t != "static" || req.UploadHash == "" || !rt.isPagesSiteApp(ctx, name, image) {
		return msgSiteTokenBuild
	}
	if req.SourceURL != "" || req.RepoURL != "" || req.InlineSource != nil || req.ImageRepo != "" || req.RerunOf != "" ||
		req.Override || req.Reason != "" || req.NoCache || req.FromCache || b.Image != "" || len(b.Args) > 0 {
		return msgSiteTokenBuild
	}
	return ""
}

// revokeSiteTokens drops every token scoped to a site and its scope rows.
func (rt *Router) revokeSiteTokens(ctx context.Context, site string) {
	ss := rt.tokenSiteScopes()
	if ss == nil {
		return
	}
	ids, err := ss.ListAPITokenIDsForSite(ctx, site)
	if err != nil {
		rt.logger.Warn("api: list site tokens for cleanup failed", slog.String("site", site), slog.String("error", err.Error()))
		return
	}
	for _, id := range ids {
		if err := rt.libraryRevokeToken(ctx, id); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) && !isTokenGone(err) {
			rt.logger.Warn("api: revoke site token failed", slog.String("token_id", id), slog.String("error", err.Error()))
		}
	}
	if d, ok := ss.(interface {
		DeleteAPITokenSiteScopes(ctx context.Context, site string) error
	}); ok {
		if err := d.DeleteAPITokenSiteScopes(ctx, site); err != nil {
			rt.logger.Warn("api: delete site scopes failed", slog.String("site", site), slog.String("error", err.Error()))
		}
	}
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
	ownerID, sessionOK := rt.currentSessionUserID(r)
	if !rt.signedInUserSession(r) || !sessionOK {
		writeError(w, http.StatusForbidden, "site deploy tokens are minted from a signed-in session")
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
	if !rt.signedInUserSession(r) {
		writeError(w, http.StatusForbidden, "site deploy tokens are revoked from a signed-in session")
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
