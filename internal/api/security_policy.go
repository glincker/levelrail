package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envApprovalScope          = "APP_AUTH_APPROVAL_SCOPE"
	envTokenMaxLifetimeDays   = "APP_TOKEN_MAX_LIFETIME_DAYS"    //nolint:gosec // env var name, not a credential
	envTokenWarnUnusedDays    = "APP_TOKEN_WARN_UNUSED_DAYS"     //nolint:gosec // env var name, not a credential
	envTokenDisableUnusedDays = "APP_TOKEN_DISABLE_UNUSED_DAYS"  //nolint:gosec // env var name, not a credential
	envTokenHygieneGrace      = "APP_TOKEN_HYGIENE_NOTICE_GRACE" //nolint:gosec // env var name, not a credential
	defaultTokenWarnUnused    = 30
	defaultTokenHygieneGrace  = 7 * 24 * time.Hour
	maxPolicyDays             = 3650

	approvalScopePasswordOnly = "password_only"
	approvalScopeAllMethods   = "all_methods"

	signInMethodPassword = "password"
	signInMethodPasskey  = "passkey"
	signInMethodOAuth    = "oauth"

	policySourceSaved   = "saved"
	policySourceEnv     = "env"
	policySourceDefault = "default"
)

// SecurityStore is the store surface the security center needs.
type SecurityStore interface {
	GetSecurityPolicy(ctx context.Context) (store.SecurityPolicy, bool, error)
	SaveSecurityPolicy(ctx context.Context, p store.SecurityPolicy) error
	GetUserSecuritySettings(ctx context.Context, userID string) (store.UserSecuritySettings, error)
	ListUserSecuritySettings(ctx context.Context) ([]store.UserSecuritySettings, error)
	SetRequireNewDeviceApproval(ctx context.Context, userID string, on bool, now time.Time) error
	FlagUserForReset(ctx context.Context, userID, reason string, now time.Time) error
	ClearUserResetFlag(ctx context.Context, userID string, now time.Time) error
	CreateSignInAlertToken(ctx context.Context, t store.SignInAlertToken) error
	ConsumeSignInAlertToken(ctx context.Context, id string, now time.Time) (store.SignInAlertToken, error)
	PruneSignInAlertTokens(ctx context.Context, cutoff time.Time) error
	ListTokenHygieneNotices(ctx context.Context) ([]store.TokenHygieneNotice, error)
	SaveTokenHygieneNotice(ctx context.Context, n store.TokenHygieneNotice) error
	MarkTokenHygieneDisabled(ctx context.Context, tokenID string, now time.Time) error
	DeleteTokenHygieneNotice(ctx context.Context, tokenID string) error
	RememberBrowser(ctx context.Context, userID, fingerprint string, now time.Time) (bool, bool, error)
	ForgetBrowser(ctx context.Context, userID, fingerprint string) error
	PruneKnownBrowsers(ctx context.Context, cutoff time.Time) error
}

// securityState is the security center's in-memory state.
type securityState struct {
	alertKey   []byte
	posture    postureCache
	failures   *failedLoginCounter
	alertLimit *apiRateLimiter
	hygiene    hygieneClock
}

func newSecurityState() *securityState {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("api: generate sign-in alert key: %v", err))
	}
	return &securityState{
		alertKey:   key,
		failures:   newFailedLoginCounter(),
		alertLimit: newWindowRateLimiter(envInt(envSignInAlertRate, defaultSignInAlertRate), time.Hour),
	}
}

// deriveAlertKey ties "this wasn't me" links to the control plane's own key,
// so they survive a restart; without one, a random per-process key is used.
func (s *securityState) deriveAlertKey(key []byte) {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("sign-in-alert-link-v1"))
	s.alertKey = m.Sum(nil)
}

// envDays reads a day count where 0 is meaningful (off).
func envDays(name string, def int) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > maxPolicyDays {
		return def, false
	}
	return n, true
}

type securityPolicy struct {
	ApprovalScope        string `json:"approval_scope"`
	MaxTokenLifetimeDays int    `json:"max_token_lifetime_days"`
	WarnUnusedDays       int    `json:"warn_unused_days"`
	DisableUnusedDays    int    `json:"disable_unused_days"`
}

type securityPolicyResource struct {
	securityPolicy
	Sources           map[string]string `json:"sources"`
	NewDeviceApproval bool              `json:"new_device_approval"`
	NoticeGraceDays   int               `json:"notice_grace_days"`
}

type securityPolicyRequest struct {
	ApprovalScope        *string `json:"approval_scope"`
	MaxTokenLifetimeDays *int    `json:"max_token_lifetime_days"`
	WarnUnusedDays       *int    `json:"warn_unused_days"`
	DisableUnusedDays    *int    `json:"disable_unused_days"`
}

func validApprovalScope(s string) bool {
	return s == approvalScopePasswordOnly || s == approvalScopeAllMethods
}

func envApprovalScopeDefault() (string, bool) {
	v := strings.TrimSpace(os.Getenv(envApprovalScope))
	if validApprovalScope(v) {
		return v, true
	}
	return approvalScopePasswordOnly, false
}

// effectiveSecurityPolicy layers the stored policy over the env defaults.
func (rt *Router) effectiveSecurityPolicy(ctx context.Context) (securityPolicy, map[string]string, error) {
	sources := map[string]string{}
	src := func(key string, fromEnv bool) {
		sources[key] = policySourceDefault
		if fromEnv {
			sources[key] = policySourceEnv
		}
	}
	var p securityPolicy
	var fromEnv bool
	p.ApprovalScope, fromEnv = envApprovalScopeDefault()
	src("approval_scope", fromEnv)
	p.MaxTokenLifetimeDays, fromEnv = envDays(envTokenMaxLifetimeDays, 0)
	src("max_token_lifetime_days", fromEnv)
	p.WarnUnusedDays, fromEnv = envDays(envTokenWarnUnusedDays, defaultTokenWarnUnused)
	src("warn_unused_days", fromEnv)
	p.DisableUnusedDays, fromEnv = envDays(envTokenDisableUnusedDays, 0)
	src("disable_unused_days", fromEnv)
	if rt.security == nil {
		return p, sources, nil
	}
	saved, ok, err := rt.security.GetSecurityPolicy(ctx)
	if err != nil {
		return p, sources, fmt.Errorf("api: load security policy: %w", err)
	}
	if !ok {
		return p, sources, nil
	}
	if saved.ApprovalScope != nil && validApprovalScope(*saved.ApprovalScope) {
		p.ApprovalScope, sources["approval_scope"] = *saved.ApprovalScope, policySourceSaved
	}
	for _, f := range []struct {
		key string
		in  *int
		out *int
	}{
		{"max_token_lifetime_days", saved.MaxTokenLifetimeDays, &p.MaxTokenLifetimeDays},
		{"warn_unused_days", saved.WarnUnusedDays, &p.WarnUnusedDays},
		{"disable_unused_days", saved.DisableUnusedDays, &p.DisableUnusedDays},
	} {
		if f.in != nil {
			*f.out, sources[f.key] = *f.in, policySourceSaved
		}
	}
	return p, sources, nil
}

func (rt *Router) securityPolicyResource(ctx context.Context) (securityPolicyResource, error) {
	p, sources, err := rt.effectiveSecurityPolicy(ctx)
	if err != nil {
		return securityPolicyResource{}, err
	}
	return securityPolicyResource{securityPolicy: p, Sources: sources, NewDeviceApproval: rt.newDeviceApproval,
		NoticeGraceDays: int(tokenHygieneGrace().Hours() / 24)}, nil
}

func tokenHygieneGrace() time.Duration {
	return envDuration(envTokenHygieneGrace, defaultTokenHygieneGrace)
}

// handleGetSecurityPolicy handles GET /api/v1/security/policy.
func (rt *Router) handleGetSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	out, err := rt.securityPolicyResource(r.Context())
	if err != nil {
		rt.internalError(w, "api: get security policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutSecurityPolicy handles PUT /api/v1/security/policy. An omitted
// field keeps its current stored value.
func (rt *Router) handlePutSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	var req securityPolicyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.ApprovalScope != nil && !validApprovalScope(*req.ApprovalScope) {
		writeError(w, http.StatusBadRequest, "approval_scope must be "+approvalScopePasswordOnly+" or "+approvalScopeAllMethods)
		return
	}
	for name, v := range map[string]*int{"max_token_lifetime_days": req.MaxTokenLifetimeDays, "warn_unused_days": req.WarnUnusedDays, "disable_unused_days": req.DisableUnusedDays} {
		if v != nil && (*v < 0 || *v > maxPolicyDays) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s must be from 0 to %d", name, maxPolicyDays))
			return
		}
	}
	ctx := r.Context()
	saved, _, err := rt.security.GetSecurityPolicy(ctx)
	if err != nil {
		rt.internalError(w, "api: put security policy: load failed", err)
		return
	}
	if req.ApprovalScope != nil {
		saved.ApprovalScope = req.ApprovalScope
	}
	if req.MaxTokenLifetimeDays != nil {
		saved.MaxTokenLifetimeDays = req.MaxTokenLifetimeDays
	}
	if req.WarnUnusedDays != nil {
		saved.WarnUnusedDays = req.WarnUnusedDays
	}
	if req.DisableUnusedDays != nil {
		saved.DisableUnusedDays = req.DisableUnusedDays
	}
	saved.UpdatedAt = time.Now()
	if err := rt.security.SaveSecurityPolicy(ctx, saved); err != nil {
		rt.internalError(w, "api: save security policy failed", err)
		return
	}
	rt.sec.posture.invalidate()
	actorID, actorKind := rt.securityActor(r)
	rt.auditSignIn(ctx, r, signInActor{kind: actorKind, id: actorID, name: rt.auditActorName(ctx, actorKind, actorID, "")},
		store.AuditActionSecurityPolicyUpdate, "/api/v1/security/policy", http.StatusOK)
	rt.handleGetSecurityPolicy(w, r)
}

// securityActor names the caller for an audit row: its session user or token.
func (rt *Router) securityActor(r *http.Request) (id, kind string) {
	if userID, ok := rt.currentSessionUserID(r); ok {
		return userID, auditActorSession
	}
	if rec, ok := r.Context().Value(tokenIdentityKey{}).(*store.APIToken); ok {
		return rec.ID, "token"
	}
	return "", auditActorSystem
}

// approvalRequiredFor reports whether a sign-in by method for userID goes
// through the new-browser gate. Password always does; passkey and OAuth do
// under all_methods or the account's own switch. Unreadable policy fails closed.
func (rt *Router) approvalRequiredFor(ctx context.Context, userID, method string) bool {
	if method == signInMethodPassword {
		return true
	}
	p, _, err := rt.effectiveSecurityPolicy(ctx)
	if err != nil {
		rt.logger.Warn("api: approval scope unreadable, gating sign-in", slog.String("user_id", userID), slog.String("error", err.Error()))
		return true
	}
	if p.ApprovalScope == approvalScopeAllMethods {
		return true
	}
	if rt.security == nil {
		return false
	}
	us, err := rt.security.GetUserSecuritySettings(ctx, userID)
	if err != nil {
		rt.logger.Warn("api: account approval setting unreadable, gating sign-in", slog.String("user_id", userID), slog.String("error", err.Error()))
		return true
	}
	return us.RequireNewDeviceApproval
}
