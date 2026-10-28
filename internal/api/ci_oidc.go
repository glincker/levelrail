package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ghactions"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	ciTokenTTL          = 15 * time.Minute
	ciExchangeBodyLimit = 16 << 10
	maxCIRefPatternLen  = 200
)

var (
	ciRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	errCINoDashboardURL = errors.New("set the dashboard URL in Settings first, it is the audience CI tokens are issued for")
)

// CIOIDCTrustStore is the store surface for CI OIDC trusts.
type CIOIDCTrustStore interface {
	SaveCIOIDCTrust(ctx context.Context, t store.CIOIDCTrust) error
	ListCIOIDCTrusts(ctx context.Context, serviceName string) ([]store.CIOIDCTrust, error)
	ListCIOIDCTrustsForRepo(ctx context.Context, repository, serviceName string) ([]store.CIOIDCTrust, error)
	DeleteCIOIDCTrust(ctx context.Context, serviceName, id string) (store.CIOIDCTrust, error)
}

// CIClaims are the verified claims of a CI provider identity token.
type CIClaims struct {
	Repository  string
	Ref         string
	Environment string
	Subject     string
	// Provider is empty for GitHub; Issuer, RefProtected and ContextIDs are set only by the GitLab and CircleCI verifiers.
	Provider     string
	Issuer       string
	RefProtected bool
	ContextIDs   []string
}

// CIIDTokenVerifier checks a CI identity token's signature, issuer, audience and expiry.
type CIIDTokenVerifier interface {
	Verify(ctx context.Context, rawToken, audience string) (CIClaims, error)
}

// SetCIIDTokenVerifier overrides the GitHub OIDC verifier, for tests.
func (rt *Router) SetCIIDTokenVerifier(v CIIDTokenVerifier) { rt.ciVerifier = v }

type ciTrustResource struct {
	ID          string    `json:"id"`
	Repository  string    `json:"repository"`
	RefPattern  string    `json:"ref_pattern"`
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
}

type ciTrustRequest struct {
	Repository  string `json:"repository"`
	RefPattern  string `json:"ref_pattern"`
	Environment string `json:"environment"`
}

func toCITrustResource(t store.CIOIDCTrust) ciTrustResource {
	return ciTrustResource{ID: t.ID, Repository: t.Repository, RefPattern: t.RefPattern, Environment: t.Environment, CreatedAt: t.CreatedAt}
}

func (rt *Router) requireAppForCI(w http.ResponseWriter, r *http.Request, name string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "get app for ci trust", err)
		}
		return false
	}
	return true
}

func validateCITrust(req *ciTrustRequest) string {
	req.Repository = strings.TrimSpace(req.Repository)
	req.RefPattern = strings.TrimSpace(req.RefPattern)
	req.Environment = strings.TrimSpace(req.Environment)
	if !ciRepositoryPattern.MatchString(req.Repository) {
		return "repository must be owner/name"
	}
	if !strings.HasPrefix(req.RefPattern, "refs/") || len(req.RefPattern) > maxCIRefPatternLen {
		return "ref_pattern must be a full ref such as refs/heads/main, refs/heads/release/* or refs/tags/v*"
	}
	if _, err := path.Match(req.RefPattern, "refs/heads/x"); err != nil {
		return "ref_pattern is not a valid glob"
	}
	if len(req.Environment) > maxGitHubEnvironmentLen {
		return "environment is too long"
	}
	return ""
}

// handleListCITrusts handles GET /api/v1/apps/{name}/ci/trusts.
func (rt *Router) handleListCITrusts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireAppForCI(w, r, name) {
		return
	}
	trusts, err := rt.ciTrusts.ListCIOIDCTrusts(r.Context(), name)
	if err != nil {
		rt.internalError(w, "list ci trusts", err)
		return
	}
	out := make([]ciTrustResource, 0, len(trusts))
	for _, t := range trusts {
		out = append(out, toCITrustResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateCITrust handles POST /api/v1/apps/{name}/ci/trusts.
func (rt *Router) handleCreateCITrust(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req ciTrustRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ciExchangeBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateCITrust(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !rt.requireAppForCI(w, r, name) {
		return
	}
	trustID, err := store.NewCIOIDCTrustID()
	if err != nil {
		rt.internalError(w, "create ci trust", err)
		return
	}
	policyID, err := rt.saveCIPolicy(r.Context(), trustID, name)
	if err != nil {
		rt.internalError(w, "create ci trust policy", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	t := store.CIOIDCTrust{
		ID: trustID, ServiceName: name, Repository: req.Repository, RefPattern: req.RefPattern,
		Environment: req.Environment, PolicyID: policyID, CreatedBy: actor, CreatedAt: time.Now().UTC(),
	}
	if err := rt.ciTrusts.SaveCIOIDCTrust(r.Context(), t); err != nil {
		_ = rt.policies.DeletePolicy(r.Context(), policyID)
		if errors.Is(err, store.ErrCIOIDCTrustExists) {
			writeError(w, http.StatusConflict, "that repository, ref and environment is already trusted for this app")
			return
		}
		rt.internalError(w, "save ci trust", err)
		return
	}
	writeJSON(w, http.StatusCreated, toCITrustResource(t))
}

// handleDeleteCITrust handles DELETE /api/v1/apps/{name}/ci/trusts/{id}.
func (rt *Router) handleDeleteCITrust(w http.ResponseWriter, r *http.Request) {
	t, err := rt.ciTrusts.DeleteCIOIDCTrust(r.Context(), r.PathValue("name"), r.PathValue("id"))
	if errors.Is(err, store.ErrCIOIDCTrustNotFound) {
		writeError(w, http.StatusNotFound, "trust not found")
		return
	}
	if err != nil {
		rt.internalError(w, "delete ci trust", err)
		return
	}
	if err := rt.policies.DeletePolicy(r.Context(), t.PolicyID); err != nil && !errors.Is(err, store.ErrPolicyNotFound) {
		rt.logger.Warn("api: delete ci trust policy failed", slog.String("trust_id", t.ID), slog.String("error", err.Error()))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) saveCIPolicy(ctx context.Context, trustID, app string) (string, error) {
	doc, err := json.Marshal(Document{Statement: []Statement{{
		Effect: EffectAllow, Action: []string{AbilityRead, AbilityDeploy}, Resource: []string{"app:" + app},
	}}})
	if err != nil {
		return "", fmt.Errorf("api: marshal ci policy: %w", err)
	}
	id, err := store.NewPolicyID()
	if err != nil {
		return "", fmt.Errorf("api: ci policy id: %w", err)
	}
	now := time.Now().UTC()
	p := store.Policy{ID: id, Name: "ci-oidc-" + trustID, Description: "Deploy " + app + " from a trusted CI workflow", Document: string(doc), CreatedAt: now, UpdatedAt: now}
	if err := rt.policies.SavePolicy(ctx, p); err != nil {
		return "", fmt.Errorf("api: save ci policy: %w", err)
	}
	return id, nil
}

func ciTrustMatches(t store.CIOIDCTrust, c CIClaims) bool {
	if t.Repository != c.Repository {
		return false
	}
	if ok, err := path.Match(t.RefPattern, c.Ref); err != nil || !ok {
		return false
	}
	return t.Environment == "" || t.Environment == c.Environment
}

type ciExchangeRequest struct {
	App     string `json:"app"`
	IDToken string `json:"id_token"`
}

type ciExchangeResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleCIOIDCExchange handles POST /api/v1/ci/oidc-exchange: it swaps a verified CI identity
// token for a short-lived token that can only read and deploy the one app the matching trust names.
func (rt *Router) handleCIOIDCExchange(w http.ResponseWriter, r *http.Request) {
	if !rt.allowTokenRedeem(w, r, "ci-oidc-exchange") {
		return
	}
	var req ciExchangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ciExchangeBodyLimit)).Decode(&req); err != nil || req.App == "" || req.IDToken == "" {
		writeError(w, http.StatusBadRequest, "app and id_token are required")
		return
	}
	audience, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.internalError(w, "ci exchange: dashboard url", err)
		return
	}
	if audience == "" {
		writeError(w, http.StatusConflict, errCINoDashboardURL.Error())
		return
	}
	verifier := rt.ciVerifier
	if verifier == nil {
		verifier = defaultGitHubOIDCVerifier
	}
	claims, err := verifier.Verify(r.Context(), req.IDToken, audience)
	if err != nil {
		rt.logger.Info("api: ci exchange: token rejected", slog.String("app", req.App), slog.String("error", err.Error()))
		writeError(w, http.StatusUnauthorized, "identity token rejected")
		return
	}
	match, err := rt.authorizeCIExchange(r.Context(), claims, req.App)
	if err != nil {
		rt.internalError(w, "ci exchange: list trusts", err)
		return
	}
	if match == nil {
		rt.logger.Info("api: ci exchange: no trust", slog.String("app", req.App), slog.String("repository", claims.Repository), slog.String("ref", claims.Ref))
		writeError(w, http.StatusForbidden, "no CI trust on this app matches the repository, ref and environment of the workflow")
		return
	}
	expires := time.Now().Add(ciTokenTTL)
	raw, rec, err := MintAPIToken(r.Context(), rt.tokens, "ci:"+claims.Repository+"@"+claims.Ref, nil, &expires)
	if err != nil {
		rt.internalError(w, "ci exchange: mint token", err)
		return
	}
	attachID, err := store.NewPolicyAttachmentID()
	if err == nil {
		err = rt.policies.AttachPolicy(r.Context(), attachID, match.PolicyID, store.PrincipalTypeToken, rec.ID)
	}
	if err != nil {
		_ = rt.tokens.RevokeAPIToken(r.Context(), rec.ID)
		rt.internalError(w, "ci exchange: scope token", err)
		return
	}
	rt.logger.Info("api: ci exchange: token issued", slog.String("app", req.App), slog.String("repository", claims.Repository), slog.String("ref", claims.Ref), slog.String("subject", claims.Subject), slog.String("token_id", rec.ID))
	writeJSON(w, http.StatusOK, ciExchangeResponse{Token: raw, ExpiresAt: expires})
}

// authorizeCIExchange returns the trust that lets these verified claims act on app, or nil.
func (rt *Router) authorizeCIExchange(ctx context.Context, claims CIClaims, app string) (*store.CIOIDCTrust, error) {
	trusts, err := rt.ciTrusts.ListCIOIDCTrustsForRepo(ctx, claims.Repository, app)
	if err != nil {
		return nil, err
	}
	for i := range trusts {
		if ciTrustMatches(trusts[i], claims) {
			return &trusts[i], nil
		}
	}
	return nil, nil
}

type ciWorkflowResource struct {
	YAML        string `json:"yaml"`
	APIURL      string `json:"api_url"`
	HasTrust    bool   `json:"has_trust"`
	NeedsDomain bool   `json:"needs_dashboard_url"`
}

// handleGetCIWorkflow handles GET /api/v1/apps/{name}/ci/workflow: a ready to paste GitHub Actions
// workflow that deploys on push with the job OIDC identity, honoring the app's path filters.
func (rt *Router) handleGetCIWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireAppForCI(w, r, name) {
		return
	}
	apiURL, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.internalError(w, "ci workflow: dashboard url", err)
		return
	}
	opts := ghactions.WorkflowOptions{App: name, APIURL: apiURL, ActionRef: githubRepo + "/integrations/github-action@main"}
	if apiURL == "" {
		opts.APIURL = "https://your-control-plane.example.com"
	}
	if gs, err := rt.gitSources.GetGitSource(r.Context(), name); err == nil && gs != nil {
		opts.Branch, opts.Paths, opts.IgnorePaths = gs.Branch, gs.DeployPaths, gs.DeployPathsIgnore
	}
	if s, err := rt.githubActions.GetAppGitHubActions(r.Context(), name); err == nil {
		opts.Environment = s.Environment
	}
	trusts, err := rt.ciTrusts.ListCIOIDCTrusts(r.Context(), name)
	if err != nil {
		rt.internalError(w, "ci workflow: list trusts", err)
		return
	}
	writeJSON(w, http.StatusOK, ciWorkflowResource{YAML: ghactions.Workflow(opts), APIURL: apiURL, HasTrust: len(trusts) > 0, NeedsDomain: apiURL == ""})
}
