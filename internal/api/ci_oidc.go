package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ghactions"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	ciTokenTTL          = 15 * time.Minute
	ciExchangeBodyLimit = 16 << 10
	maxCIRefPatternLen  = 200
	maxCIIssuerLen      = 300
)

var (
	ciRepositoryPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	gitLabProjectPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)
	ciUUIDPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	errCINoDashboardURL  = errors.New("set the dashboard URL in Settings first, it is the audience CI tokens are issued for")
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
	// Provider is empty for GitHub; the fields below are set only by the GitLab and CircleCI verifiers.
	Provider             string
	Issuer               string
	RefProtected         bool
	EnvironmentProtected bool
	ContextIDs           []string
}

// CIIDTokenVerifier checks a CI identity token's signature, issuer, audience and expiry.
type CIIDTokenVerifier interface {
	Verify(ctx context.Context, rawToken, audience string) (CIClaims, error)
}

// SetCIIDTokenVerifier overrides the GitHub OIDC verifier, for tests.
func (rt *Router) SetCIIDTokenVerifier(v CIIDTokenVerifier) { rt.ciVerifier = v }

type ciTrustResource struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	Issuer           string    `json:"issuer,omitempty"`
	Repository       string    `json:"repository"`
	RefPattern       string    `json:"ref_pattern"`
	Environment      string    `json:"environment"`
	ProtectedRefOnly bool      `json:"protected_ref_only,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	Warning          string    `json:"warning,omitempty"`
}

// ciTrustRequest: repository is owner/name on GitHub, the project path on GitLab and the project ID on CircleCI.
type ciTrustRequest struct {
	Provider    string `json:"provider,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	OrgID       string `json:"org_id,omitempty"`
	Repository  string `json:"repository"`
	RefPattern  string `json:"ref_pattern"`
	Environment string `json:"environment"`
	// ProtectedRefOnly is a pointer so an omitted value takes the provider default (true for GitLab).
	ProtectedRefOnly *bool `json:"protected_ref_only,omitempty"`
}

const ciGitLabUnprotectedWarning = "this trust accepts pipelines on unprotected branches and tags: any developer who can push a matching ref can deploy"

func (req ciTrustRequest) protectedRefOnly() bool {
	return req.ProtectedRefOnly != nil && *req.ProtectedRefOnly
}

func toCITrustResource(t store.CIOIDCTrust) ciTrustResource {
	return ciTrustResource{
		ID: t.ID, Provider: ciProviderName(t.Provider), Issuer: t.Issuer, Repository: t.Repository, RefPattern: t.RefPattern,
		Environment: t.Environment, ProtectedRefOnly: t.ProtectedRefOnly, CreatedAt: t.CreatedAt,
	}
}

func ciProviderName(p string) string {
	if p == "" {
		return ciProviderGitHub
	}
	return p
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
	req.Provider = ciProviderName(strings.ToLower(strings.TrimSpace(req.Provider)))
	req.Issuer = strings.TrimSpace(req.Issuer)
	req.OrgID = strings.TrimSpace(req.OrgID)
	req.Repository = strings.TrimSpace(req.Repository)
	req.RefPattern = strings.TrimSpace(req.RefPattern)
	req.Environment = strings.TrimSpace(req.Environment)
	if msg := validateCIProvider(req); msg != "" {
		return msg
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

// validateCIProvider fixes the issuer at configuration time: exchanges only ever fetch keys from an issuer set here.
func validateCIProvider(req *ciTrustRequest) string {
	switch req.Provider {
	case ciProviderGitHub:
		if req.Issuer != "" || req.OrgID != "" || req.protectedRefOnly() {
			return "issuer, org_id and protected_ref_only apply to gitlab and circleci trusts only"
		}
		if !ciRepositoryPattern.MatchString(req.Repository) {
			return "repository must be owner/name"
		}
	case ciProviderGitLab:
		if req.OrgID != "" {
			return "org_id applies to circleci trusts only"
		}
		if req.ProtectedRefOnly == nil {
			req.ProtectedRefOnly = new(true)
		}
		if req.Issuer == "" {
			req.Issuer = gitLabDefaultIssuer
		}
		iss, msg := normalizeCIIssuer(req.Issuer)
		if msg != "" {
			return msg
		}
		req.Issuer = iss
		if !gitLabProjectPattern.MatchString(req.Repository) || len(req.Repository) > maxCIRefPatternLen {
			return "repository must be the GitLab project path, such as group/project or group/subgroup/project"
		}
	case ciProviderCircleCI:
		if req.protectedRefOnly() {
			return "protected_ref_only applies to gitlab trusts only"
		}
		if !ciUUIDPattern.MatchString(req.OrgID) {
			return "org_id must be the CircleCI organization ID (Organization Settings > Overview)"
		}
		iss := circleCIIssuerBase + req.OrgID
		if req.Issuer != "" && req.Issuer != iss {
			return "issuer for circleci must be " + iss
		}
		req.Issuer, req.OrgID = iss, ""
		if !ciUUIDPattern.MatchString(req.Repository) {
			return "repository must be the CircleCI project ID (Project Settings > Overview)"
		}
		if req.Environment != "" && !ciUUIDPattern.MatchString(req.Environment) {
			return "environment for circleci must be a context ID the job runs with"
		}
	default:
		return "provider must be github, gitlab or circleci"
	}
	return ""
}

// normalizeCIIssuer accepts a plain https issuer URL; a self-managed GitLab may live under a path.
func normalizeCIIssuer(raw string) (string, string) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > maxCIIssuerLen {
		return "", "issuer must be an https URL such as https://gitlab.com or https://gitlab.example.com"
	}
	if h := strings.ToLower(u.Hostname()); h == "token.actions.githubusercontent.com" || h == "oidc.circleci.com" {
		return "", "issuer belongs to another CI provider"
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), ""
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
	if ok, msg := rt.ciTrustCallerMay(r, name, req); !ok {
		writeError(w, http.StatusForbidden, msg)
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
		Provider: req.Provider, Issuer: req.Issuer, ProtectedRefOnly: req.protectedRefOnly(),
	}
	if err := rt.ciTrusts.SaveCIOIDCTrust(r.Context(), t); err != nil {
		_ = rt.policies.DeletePolicy(r.Context(), policyID)
		if errors.Is(err, store.ErrCIOIDCTrustExists) {
			writeError(w, http.StatusConflict, "that provider, issuer, repository, ref and environment is already trusted for this app")
			return
		}
		rt.internalError(w, "save ci trust", err)
		return
	}
	rt.auditCITrust(r.Context(), r, auditActionCITrustCreated, t)
	res := toCITrustResource(t)
	if t.Provider == ciProviderGitLab && !t.ProtectedRefOnly {
		res.Warning = ciGitLabUnprotectedWarning
	}
	writeJSON(w, http.StatusCreated, res)
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
	rt.auditCITrust(r.Context(), r, auditActionCITrustDeleted, t)
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

// ciTrustMatches is the policy gate: provider, issuer, repository, ref and environment must all match, else deny.
func ciTrustMatches(t store.CIOIDCTrust, c CIClaims) bool {
	provider := ciProviderName(t.Provider)
	if provider != ciProviderName(c.Provider) || t.Repository != c.Repository || c.Ref == "" {
		return false
	}
	if provider != ciProviderGitHub && (t.Issuer == "" || t.Issuer != c.Issuer) {
		return false
	}
	if ok, err := path.Match(t.RefPattern, c.Ref); err != nil || !ok {
		return false
	}
	if t.ProtectedRefOnly && !c.RefProtected {
		return false
	}
	if t.Environment == "" {
		return true
	}
	switch provider {
	case ciProviderCircleCI:
		return slices.Contains(c.ContextIDs, t.Environment)
	case ciProviderGitLab:
		// Any job may declare an environment; only a protected one limits who can run it.
		return c.EnvironmentProtected && t.Environment == c.Environment
	}
	return t.Environment == c.Environment
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
	claims, candidates, err := rt.verifyCIIdentity(r.Context(), req.App, req.IDToken, audience)
	var storeErr *ciTrustLookupError
	if errors.As(err, &storeErr) {
		rt.internalError(w, "ci exchange: list trusts", storeErr.err)
		return
	}
	if err != nil {
		rt.logger.Info("api: ci exchange: token rejected", slog.String("app", req.App), slog.String("error", err.Error()))
		rt.auditCIExchange(r.Context(), r, ciExchangeAuditDetail{App: clipClaim(req.App), Provider: ciUnverifiedProvider(req.IDToken), Reason: ciExchangeReasonRejected})
		writeError(w, http.StatusUnauthorized, "identity token rejected")
		return
	}
	match := firstCITrustMatch(candidates, claims)
	if match == nil {
		rt.logger.Info("api: ci exchange: no trust", slog.String("app", req.App), slog.String("provider", ciProviderName(claims.Provider)), slog.String("subject", ciLogSubject(claims.Subject)))
		rt.auditCIExchange(r.Context(), r, ciExchangeAuditDetail{App: req.App, Provider: ciProviderName(claims.Provider), Reason: ciExchangeReasonNoTrust, Subject: ciRedactedSubject(claims.Subject)})
		writeError(w, http.StatusForbidden, "no CI trust on this app matches the repository, ref and environment of the workflow")
		return
	}
	expires := time.Now().Add(ciTokenTTL)
	label := "ci:" + claims.Repository + "@" + claims.Ref
	if p := ciProviderName(claims.Provider); p != ciProviderGitHub {
		label = "ci:" + p + ":" + claims.Repository + "@" + claims.Ref
	}
	raw, rec, err := MintAPIToken(r.Context(), rt.tokens, label, nil, &expires)
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
	rt.logger.Info("api: ci exchange: token issued", slog.String("app", req.App), slog.String("trust_id", match.ID), slog.String("provider", ciProviderName(claims.Provider)), slog.String("subject", ciLogSubject(claims.Subject)), slog.String("token_id", rec.ID))
	rt.auditCIExchange(r.Context(), r, ciExchangeAuditDetail{
		App: req.App, Provider: ciProviderName(claims.Provider), Reason: ciExchangeReasonIssued,
		Subject: ciRedactedSubject(claims.Subject), TrustID: match.ID, TokenID: rec.ID,
	})
	writeJSON(w, http.StatusOK, ciExchangeResponse{Token: raw, ExpiresAt: expires})
}

type ciTrustLookupError struct{ err error }

func (e *ciTrustLookupError) Error() string { return "api: list ci trusts: " + e.err.Error() }
func (e *ciTrustLookupError) Unwrap() error { return e.err }

// verifyCIIdentity picks the verifier by issuer: GitHub's fixed one, or a GitLab or CircleCI issuer that a trust on
// this app names exactly. Any other issuer is refused before a single key fetch.
func (rt *Router) verifyCIIdentity(ctx context.Context, app, rawToken, audience string) (CIClaims, []store.CIOIDCTrust, error) {
	iss := unverifiedCIIssuer(rawToken)
	if iss == "" || iss == githubActionsOIDCIssuer {
		verifier := rt.ciVerifier
		if verifier == nil {
			verifier = defaultGitHubOIDCVerifier
		}
		claims, err := verifier.Verify(ctx, rawToken, audience)
		if err != nil {
			return CIClaims{}, nil, err
		}
		trusts, err := rt.ciTrusts.ListCIOIDCTrustsForRepo(ctx, claims.Repository, app)
		if err != nil {
			return CIClaims{}, nil, &ciTrustLookupError{err}
		}
		return claims, trusts, nil
	}
	all, err := rt.ciTrusts.ListCIOIDCTrusts(ctx, app)
	if err != nil {
		return CIClaims{}, nil, &ciTrustLookupError{err}
	}
	var candidates []store.CIOIDCTrust
	for _, t := range all {
		if p := ciProviderName(t.Provider); p != ciProviderGitHub && t.Issuer == iss {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return CIClaims{}, nil, errors.New("api: ci oidc issuer is not configured on any trust of this app")
	}
	spec, err := ciProviderSpecFor(ciProviderName(candidates[0].Provider), candidates[0].Issuer)
	if err != nil {
		return CIClaims{}, nil, err
	}
	pv, ok := rt.ciVerifier.(ciProviderVerifier)
	if !ok {
		pv = defaultCIJWTVerifier()
	}
	claims, err := pv.VerifyProvider(ctx, rawToken, spec, audience)
	if err != nil {
		return CIClaims{}, nil, err
	}
	return claims, candidates, nil
}

func firstCITrustMatch(trusts []store.CIOIDCTrust, claims CIClaims) *store.CIOIDCTrust {
	for i := range trusts {
		if ciTrustMatches(trusts[i], claims) {
			return &trusts[i]
		}
	}
	return nil
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
