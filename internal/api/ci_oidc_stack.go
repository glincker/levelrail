package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Stack OIDC trusts (ADR 042): an admin binds a CI identity to one stack and the verbs it may
// use. The exchange mints a short-lived stack token that the iam_stack gate then confines.

const (
	defaultStackOIDCTTL     = 15 * time.Minute
	maxStackOIDCPatternLen  = 200
	maxStackOIDCClaimLogLen = 120
	stackOIDCTokenPrefix    = stackCIPrefix + "stack:"

	auditStackTrustCreate    = "stack.oidc_trust_create"
	auditStackTrustDelete    = "stack.oidc_trust_delete"
	auditStackOIDCExchange   = "stack.oidc_exchange"
	auditStackOIDCDenied     = "stack.oidc_exchange_denied"
	msgStackOIDCNoTrust      = "no OIDC trust on this stack matches the identity of the workflow"
	msgStackOIDCNeedsContext = "set the dashboard URL in Settings first, it is the audience CI tokens are issued for"
)

// stackOIDCTTL reads the minted token lifetime from APP_STACK_OIDC_TTL.
func stackOIDCTTL() time.Duration { return envDuration("APP_STACK_OIDC_TTL", defaultStackOIDCTTL) }

// CIStackClaims are the verified claims of a CI identity token, plus the run details the audit trail records.
type CIStackClaims struct {
	CIClaims
	SHA   string
	RunID string
}

// CIStackIDTokenVerifier is a CIIDTokenVerifier that also returns the commit and run of the workflow.
type CIStackIDTokenVerifier interface {
	CIIDTokenVerifier
	VerifyStack(ctx context.Context, rawToken, audience string) (CIStackClaims, error)
}

// VerifyStack checks signature, issuer, audience and expiry against GitHub's published keys.
func (v *githubOIDCVerifier) VerifyStack(ctx context.Context, rawToken, audience string) (CIStackClaims, error) {
	p, err := v.discover(ctx)
	if err != nil {
		return CIStackClaims{}, err
	}
	tok, err := p.Verifier(&oidc.Config{ClientID: audience}).Verify(ctx, rawToken)
	if err != nil {
		return CIStackClaims{}, fmt.Errorf("api: verify github oidc token: %w", err)
	}
	var c struct {
		Repository  string `json:"repository"`
		Ref         string `json:"ref"`
		Environment string `json:"environment"`
		SHA         string `json:"sha"`
		RunID       string `json:"run_id"`
	}
	if err := tok.Claims(&c); err != nil {
		return CIStackClaims{}, fmt.Errorf("api: read github oidc claims: %w", err)
	}
	return CIStackClaims{
		CIClaims: CIClaims{Repository: c.Repository, Ref: c.Ref, Environment: c.Environment, Subject: tok.Subject},
		SHA:      c.SHA, RunID: c.RunID,
	}, nil
}

// verifyStackAnyProvider picks the verifier by issuer like app exchanges: GitHub's fixed one, or a GitLab or
// CircleCI issuer that a trust on this stack names exactly. Any other issuer is refused before a key fetch.
func (rt *Router) verifyStackAnyProvider(ctx context.Context, stackName, raw, audience string) (CIStackClaims, error) {
	iss := unverifiedCIIssuer(raw)
	if iss == "" || iss == githubActionsOIDCIssuer {
		return rt.verifyStackIdentity(ctx, raw, audience)
	}
	db, ts := rt.stackGateDB(), rt.ciStackTrusts()
	if db == nil || ts == nil {
		return CIStackClaims{}, errors.New("api: stack oidc trusts are not available")
	}
	stack, err := db.GetStackByName(ctx, stackName)
	if err != nil {
		return CIStackClaims{}, errors.New("api: stack oidc issuer is not configured")
	}
	all, err := ts.ListCIStackTrusts(ctx, stack.ID)
	if err != nil {
		return CIStackClaims{}, fmt.Errorf("api: list stack trusts: %w", err)
	}
	for _, t := range all {
		if p := ciProviderName(t.Provider); p == ciProviderGitHub || t.Issuer != iss {
			continue
		}
		spec, err := ciProviderSpecFor(ciProviderName(t.Provider), t.Issuer)
		if err != nil {
			return CIStackClaims{}, err
		}
		pv, ok := rt.ciVerifier.(ciProviderVerifier)
		if !ok {
			pv = defaultCIJWTVerifier()
		}
		c, err := pv.VerifyProvider(ctx, raw, spec, audience)
		return CIStackClaims{CIClaims: c}, err
	}
	return CIStackClaims{}, errors.New("api: stack oidc issuer is not configured on any trust of this stack")
}

func (rt *Router) verifyStackIdentity(ctx context.Context, raw, audience string) (CIStackClaims, error) {
	switch v := rt.ciVerifier.(type) {
	case nil:
		return defaultGitHubOIDCVerifier.VerifyStack(ctx, raw, audience)
	case CIStackIDTokenVerifier:
		return v.VerifyStack(ctx, raw, audience)
	default:
		c, err := v.Verify(ctx, raw, audience)
		return CIStackClaims{CIClaims: c}, err
	}
}

type ciStackTrustStore interface {
	SaveCIStackTrust(ctx context.Context, t store.CIStackTrust) error
	ListCIStackTrusts(ctx context.Context, stackID string) ([]store.CIStackTrust, error)
	ListCIStackTrustsForRepo(ctx context.Context, repository, stackID string) ([]store.CIStackTrust, error)
	DeleteCIStackTrust(ctx context.Context, stackID, id string) (store.CIStackTrust, error)
}

func (rt *Router) ciStackTrusts() ciStackTrustStore {
	if s, ok := rt.policies.(ciStackTrustStore); ok {
		return s
	}
	if s, ok := rt.apps.(ciStackTrustStore); ok {
		return s
	}
	return nil
}

// globSegmentOK allows at most one "*" per "/" segment and otherwise only plain name characters,
// so a pattern can never mean more than "one path segment varies here".
func globSegmentOK(seg string) bool {
	if seg == "" || strings.Count(seg, "*") > 1 {
		return false
	}
	for _, c := range seg {
		plain := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-:@+*", c)
		if !plain {
			return false
		}
	}
	return true
}

func validGlob(p string) bool {
	if p == "" || len(p) > maxStackOIDCPatternLen {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if !globSegmentOK(seg) {
			return false
		}
	}
	return true
}

// globMatch matches value against pattern segment by segment; a "*" never crosses "/" and
// never matches an empty string. Invalid patterns match nothing.
func globMatch(pattern, value string) bool {
	if !validGlob(pattern) {
		return false
	}
	ps, vs := strings.Split(pattern, "/"), strings.Split(value, "/")
	if len(ps) != len(vs) {
		return false
	}
	for i := range ps {
		star := strings.IndexByte(ps[i], '*')
		if star < 0 {
			if ps[i] != vs[i] {
				return false
			}
			continue
		}
		pre, suf := ps[i][:star], ps[i][star+1:]
		if len(vs[i]) <= len(pre)+len(suf) || !strings.HasPrefix(vs[i], pre) || !strings.HasSuffix(vs[i], suf) {
			return false
		}
	}
	return true
}

type stackTrustRequest struct {
	Provider       string   `json:"provider,omitempty"`
	Issuer         string   `json:"issuer,omitempty"`
	OrgID          string   `json:"org_id,omitempty"`
	Repository     string   `json:"repository"`
	RefPattern     string   `json:"ref_pattern"`
	Environment    string   `json:"environment"`
	SubjectPattern string   `json:"subject_pattern"`
	Verbs          []string `json:"verbs"`
	// ProtectedRefOnly is a pointer so an omitted value takes the provider default (true for GitLab).
	ProtectedRefOnly *bool `json:"protected_ref_only,omitempty"`
}

type stackTrustResource struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	Issuer           string    `json:"issuer,omitempty"`
	ProtectedRefOnly bool      `json:"protected_ref_only,omitempty"`
	Warning          string    `json:"warning,omitempty"`
	Stack            string    `json:"stack"`
	Repository       string    `json:"repository"`
	RefPattern       string    `json:"ref_pattern"`
	Environment      string    `json:"environment"`
	SubjectPattern   string    `json:"subject_pattern"`
	Verbs            []string  `json:"verbs"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

func toStackTrustResource(stack string, t store.CIStackTrust) stackTrustResource {
	return stackTrustResource{
		ID: t.ID, Stack: stack, Provider: ciProviderName(t.Provider), Issuer: t.Issuer, ProtectedRefOnly: t.ProtectedRefOnly, Repository: t.Repository, RefPattern: t.RefPattern, Environment: t.Environment,
		SubjectPattern: t.SubjectPattern, Verbs: t.Verbs, CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt,
	}
}

// validateStackTrust normalizes req and returns a message when it is unsafe. Apply is held to a
// stricter bar than plan: an exact non-PR ref and a named environment, so a fork or PR run can never apply.
func validateStackTrust(req *stackTrustRequest) string {
	req.Repository = strings.TrimSpace(req.Repository)
	req.RefPattern = strings.TrimSpace(req.RefPattern)
	req.Environment = strings.TrimSpace(req.Environment)
	req.SubjectPattern = strings.TrimSpace(req.SubjectPattern)
	shared := ciTrustRequest{
		Provider: req.Provider, Issuer: req.Issuer, OrgID: req.OrgID, Repository: req.Repository,
		Environment: req.Environment, ProtectedRefOnly: req.ProtectedRefOnly,
	}
	if msg := validateStackProvider(&shared); msg != "" {
		return msg
	}
	req.Provider, req.Issuer, req.OrgID, req.ProtectedRefOnly, req.Environment = shared.Provider, shared.Issuer, shared.OrgID, shared.ProtectedRefOnly, shared.Environment
	if req.Provider != ciProviderGitHub && req.SubjectPattern != "" {
		return "subject_pattern applies to github trusts only"
	}
	parts := strings.Split(req.RefPattern, "/")
	if !validGlob(req.RefPattern) || len(parts) < 3 || parts[0] != "refs" || (parts[1] != "heads" && parts[1] != "tags" && parts[1] != "pull") {
		return "ref_pattern must be a ref such as refs/heads/main, refs/heads/release/* or refs/pull/*/merge, with at most one * per segment"
	}
	if strings.Contains(req.Environment, "*") || len(req.Environment) > maxGitHubEnvironmentLen {
		return "environment must be an exact name"
	}
	if req.Provider != ciProviderGitHub && parts[1] == "pull" {
		return "gitlab and circleci trusts take branch or tag refs only"
	}
	if req.SubjectPattern != "" && (!validGlob(req.SubjectPattern) || !strings.HasPrefix(req.SubjectPattern, "repo:"+req.Repository+":")) {
		return "subject_pattern must start with repo:<repository>: and hold at most one * per segment"
	}
	if len(req.Verbs) == 0 {
		req.Verbs = []string{store.StackVerbPlan}
	}
	if !store.ValidStackVerbs(req.Verbs) {
		return "verbs may only hold plan and apply"
	}
	req.Verbs = dedupe(req.Verbs)
	for _, v := range req.Verbs {
		if v != store.StackVerbApply {
			continue
		}
		if parts[1] == "pull" || strings.Contains(req.RefPattern, "*") {
			return "an apply trust needs an exact branch or tag ref, never a pull request or a wildcard"
		}
		if req.Environment == "" {
			return "an apply trust needs an environment"
		}
		if req.Provider == ciProviderGitLab && (req.ProtectedRefOnly == nil || !*req.ProtectedRefOnly) {
			return "a gitlab apply trust needs protected_ref_only"
		}
	}
	return ""
}

// validateStackProvider runs the app-trust provider rules; GitHub keeps the stricter exact owner/name form.
func validateStackProvider(req *ciTrustRequest) string {
	req.Provider = ciProviderName(strings.ToLower(strings.TrimSpace(req.Provider)))
	req.Issuer, req.OrgID = strings.TrimSpace(req.Issuer), strings.TrimSpace(req.OrgID)
	return validateCIProvider(req)
}

// stackForTrust resolves the stack of a trust route, writing the error itself.
func (rt *Router) stackForTrust(w http.ResponseWriter, r *http.Request) (store.StackRecord, ciStackTrustStore, bool) {
	db, ts := rt.stackGateDB(), rt.ciStackTrusts()
	if db == nil || ts == nil {
		writeError(w, http.StatusNotImplemented, "stack OIDC trusts are not available on this store")
		return store.StackRecord{}, nil, false
	}
	stack, err := db.GetStackByName(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrStackNotFound) {
		writeError(w, http.StatusNotFound, "stack not found")
		return store.StackRecord{}, nil, false
	}
	if err != nil {
		rt.internalError(w, "stack oidc trust: get stack", err)
		return store.StackRecord{}, nil, false
	}
	return stack, ts, true
}

// handleCreateStackTrust handles POST /api/v1/stacks/{name}/oidc-trusts (root).
func (rt *Router) handleCreateStackTrust(w http.ResponseWriter, r *http.Request) {
	stack, ts, ok := rt.stackForTrust(w, r)
	if !ok {
		return
	}
	var req stackTrustRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ciExchangeBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateStackTrust(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	id, err := store.NewCIStackTrustID()
	if err != nil {
		rt.internalError(w, "stack oidc trust: id", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	t := store.CIStackTrust{
		ID: id, StackID: stack.ID, Repository: req.Repository, RefPattern: req.RefPattern, Environment: req.Environment,
		SubjectPattern: req.SubjectPattern, Verbs: req.Verbs, CreatedBy: actor, CreatedAt: time.Now().UTC(),
		Provider: req.Provider, Issuer: req.Issuer, ProtectedRefOnly: req.ProtectedRefOnly != nil && *req.ProtectedRefOnly,
	}
	if err := ts.SaveCIStackTrust(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrCIStackTrustExists) {
			writeError(w, http.StatusConflict, "that repository, ref, environment and subject is already trusted for this stack")
			return
		}
		rt.internalError(w, "stack oidc trust: save", err)
		return
	}
	rt.auditStack(r, auditStackTrustCreate, map[string]any{
		"stack": stack.Name, "trust_id": t.ID, "repository": t.Repository, "ref_pattern": t.RefPattern,
		"environment": t.Environment, "subject_pattern": t.SubjectPattern, "verbs": t.Verbs,
		"provider": ciProviderName(t.Provider), "issuer": t.Issuer, "protected_ref_only": t.ProtectedRefOnly,
	})
	res := toStackTrustResource(stack.Name, t)
	if t.Provider == ciProviderGitLab && !t.ProtectedRefOnly {
		res.Warning = ciGitLabUnprotectedWarning
	}
	writeJSON(w, http.StatusCreated, res)
}

// handleListStackTrusts handles GET /api/v1/stacks/{name}/oidc-trusts (root).
func (rt *Router) handleListStackTrusts(w http.ResponseWriter, r *http.Request) {
	stack, ts, ok := rt.stackForTrust(w, r)
	if !ok {
		return
	}
	trusts, err := ts.ListCIStackTrusts(r.Context(), stack.ID)
	if err != nil {
		rt.internalError(w, "stack oidc trust: list", err)
		return
	}
	out := make([]stackTrustResource, 0, len(trusts))
	for _, t := range trusts {
		out = append(out, toStackTrustResource(stack.Name, t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"trusts": out})
}

// handleDeleteStackTrust handles DELETE /api/v1/stacks/{name}/oidc-trusts/{id} (root).
func (rt *Router) handleDeleteStackTrust(w http.ResponseWriter, r *http.Request) {
	stack, ts, ok := rt.stackForTrust(w, r)
	if !ok {
		return
	}
	t, err := ts.DeleteCIStackTrust(r.Context(), stack.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrCIStackTrustNotFound) {
		writeError(w, http.StatusNotFound, "trust not found")
		return
	}
	if err != nil {
		rt.internalError(w, "stack oidc trust: delete", err)
		return
	}
	rt.auditStack(r, auditStackTrustDelete, map[string]any{
		"stack": stack.Name, "trust_id": t.ID, "repository": t.Repository, "ref_pattern": t.RefPattern, "environment": t.Environment,
	})
	w.WriteHeader(http.StatusNoContent)
}

// stackTrustMatches reports whether every claim constraint of t holds. An empty environment
// constraint is open, but only plan trusts can be saved that way.
func stackTrustMatches(t store.CIStackTrust, c CIClaims) bool {
	if !ciIdentityAllowed(t.Provider, t.Issuer, t.Repository, t.Environment, t.ProtectedRefOnly, c) || !globMatch(t.RefPattern, c.Ref) {
		return false
	}
	return t.SubjectPattern == "" || globMatch(t.SubjectPattern, c.Subject)
}

type stackExchangeRequest struct {
	Stack   string `json:"stack"`
	IDToken string `json:"id_token"`
}

type stackExchangeResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Stack     string    `json:"stack"`
	Verbs     []string  `json:"verbs"`
}

func clipClaim(s string) string {
	if len(s) > maxStackOIDCClaimLogLen {
		return s[:maxStackOIDCClaimLogLen]
	}
	return s
}

// auditStackExchange records an exchange by the CI identity itself; the token secret never enters the detail.
func (rt *Router) auditStackExchange(r *http.Request, action, actorID, actorName string, detail map[string]any) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: stack oidc audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: store.PrincipalTypeToken, ActorID: actorID, ActorName: actorName,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: stack oidc audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

func (rt *Router) denyStackExchange(w http.ResponseWriter, r *http.Request, stack string, c CIStackClaims, reason string) {
	rt.auditStackExchange(r, auditStackOIDCDenied, "", "ci:stack:denied", map[string]any{
		"stack": clipClaim(stack), "reason": reason, "repository": clipClaim(c.Repository), "ref": clipClaim(c.Ref),
		"environment": clipClaim(c.Environment), "sha": clipClaim(c.SHA), "run_id": clipClaim(c.RunID),
	})
	writeError(w, http.StatusForbidden, msgStackOIDCNoTrust)
}

// handleStackOIDCExchange handles POST /api/v1/ci/stack-oidc-exchange: it swaps a verified CI identity
// token for a short-lived token bound to one stack with only the verbs the one matching trust allows.
// Every miss is the same 403 so a caller cannot probe which stacks or trusts exist.
func (rt *Router) handleStackOIDCExchange(w http.ResponseWriter, r *http.Request) {
	if !rt.allowTokenRedeem(w, r, "ci-stack-oidc-exchange") {
		return
	}
	var req stackExchangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, ciExchangeBodyLimit)).Decode(&req); err != nil || req.Stack == "" || req.IDToken == "" {
		writeError(w, http.StatusBadRequest, "stack and id_token are required")
		return
	}
	audience, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.internalError(w, "stack oidc exchange: dashboard url", err)
		return
	}
	if audience == "" {
		writeError(w, http.StatusConflict, msgStackOIDCNeedsContext)
		return
	}
	claims, err := rt.verifyStackAnyProvider(r.Context(), req.Stack, req.IDToken, audience)
	if err != nil {
		rt.logger.Info("api: stack oidc exchange: token rejected", slog.String("stack", clipClaim(req.Stack)), slog.String("error", err.Error()))
		writeError(w, http.StatusUnauthorized, "identity token rejected")
		return
	}
	db, ts := rt.stackGateDB(), rt.ciStackTrusts()
	if db == nil || ts == nil || rt.tokenStackScopes() == nil {
		writeError(w, http.StatusForbidden, msgStackOIDCNoTrust)
		return
	}
	if claims.Repository == "" || claims.Ref == "" {
		rt.denyStackExchange(w, r, req.Stack, claims, "missing repository or ref claim")
		return
	}
	stack, err := db.GetStackByName(r.Context(), req.Stack)
	if errors.Is(err, store.ErrStackNotFound) {
		rt.denyStackExchange(w, r, req.Stack, claims, "unknown stack")
		return
	}
	if err != nil {
		rt.internalError(w, "stack oidc exchange: get stack", err)
		return
	}
	trusts, err := ts.ListCIStackTrustsForRepo(r.Context(), claims.Repository, stack.ID)
	if err != nil {
		rt.internalError(w, "stack oidc exchange: list trusts", err)
		return
	}
	var match *store.CIStackTrust
	for i := range trusts {
		if !stackTrustMatches(trusts[i], claims.CIClaims) {
			continue
		}
		if match != nil {
			rt.denyStackExchange(w, r, stack.Name, claims, "ambiguous trusts")
			return
		}
		match = &trusts[i]
	}
	if match == nil {
		rt.denyStackExchange(w, r, stack.Name, claims, "no matching trust")
		return
	}
	rt.mintStackOIDCToken(w, r, stack, *match, claims)
}

func (rt *Router) mintStackOIDCToken(w http.ResponseWriter, r *http.Request, stack store.StackRecord, trust store.CIStackTrust, claims CIStackClaims) {
	abilities := stackTrustAbilities(trust.Verbs)
	expires := time.Now().Add(stackOIDCTTL())
	name := stackOIDCTokenPrefix + stack.Name
	raw, rec, err := MintAPIToken(r.Context(), rt.tokens, name, abilities, &expires)
	if err != nil {
		rt.internalError(w, "stack oidc exchange: mint token", err)
		return
	}
	err = rt.tokenStackScopes().SetAPITokenStackScope(r.Context(), store.APITokenStackScope{TokenID: rec.ID, StackID: stack.ID, Verbs: trust.Verbs})
	if err != nil {
		// A token without its stack scope would be unconfined, so it never leaves this function.
		_ = rt.tokens.RevokeAPIToken(r.Context(), rec.ID)
		rt.internalError(w, "stack oidc exchange: scope token", err)
		return
	}
	rt.auditStackExchange(r, auditStackOIDCExchange, rec.ID, name, map[string]any{
		"stack": stack.Name, "trust_id": trust.ID, "verbs": trust.Verbs, "token_id": rec.ID, "expires_at": expires.UTC().Format(time.RFC3339),
		"repository": clipClaim(claims.Repository), "ref": clipClaim(claims.Ref), "environment": clipClaim(claims.Environment),
		"sha": clipClaim(claims.SHA), "run_id": clipClaim(claims.RunID), "subject": clipClaim(claims.Subject),
		"provider": ciProviderName(claims.Provider),
	})
	writeJSON(w, http.StatusOK, stackExchangeResponse{Token: raw, ExpiresAt: expires, Stack: stack.Name, Verbs: trust.Verbs})
}

// stackTrustAbilities is the ability set the verbs need and nothing more.
func stackTrustAbilities(verbs []string) []string {
	var out []string
	for _, v := range verbs {
		out = append(out, stackVerbAbilities[v]...)
	}
	return dedupe(out)
}
