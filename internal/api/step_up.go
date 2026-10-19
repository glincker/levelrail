package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

// Step-up scopes: a grant covers one action on one app, never more.
const (
	stepUpActionReveal = "reveal"
	stepUpActionExport = "export"
	stepUpActionManage = "manage"

	stepUpMethodPassword = "password"
	stepUpMethodPasskey  = "passkey"
	stepUpMethodPolicy   = "policy_allow"
	stepUpMethodGrant    = "grant"

	auditActionStepUpVerified = "step_up.verified"
	auditActionStepUpFailed   = "step_up.failed"
	auditActionStepUpLocked   = "step_up.locked"
	auditActionStepUpNeedCode = "step_up.code_required"
)

var validStepUpActions = map[string]bool{stepUpActionReveal: true, stepUpActionExport: true, stepUpActionManage: true}

type stepUpGrant struct {
	userID  string
	method  string
	expires time.Time
}

// stepUpGrants holds short-lived grants keyed by a hash of the session token
// and the scope, so a grant is useless from any other session or scope.
type stepUpGrants struct {
	mu sync.Mutex
	m  map[string]stepUpGrant
}

func newStepUpGrants() *stepUpGrants {
	return &stepUpGrants{m: map[string]stepUpGrant{}}
}

func stepUpGrantKey(sessionToken, action, app string) string {
	sum := sha256.Sum256([]byte(sessionToken + "\x00" + action + "\x00" + app))
	return hex.EncodeToString(sum[:])
}

func (g *stepUpGrants) issue(sessionToken, userID, action, app, method string, ttl time.Duration, now time.Time) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, v := range g.m {
		if now.After(v.expires) {
			delete(g.m, k)
		}
	}
	exp := now.Add(ttl)
	g.m[stepUpGrantKey(sessionToken, action, app)] = stepUpGrant{userID: userID, method: method, expires: exp}
	return exp
}

func (g *stepUpGrants) check(sessionToken, userID, action, app string, now time.Time) (stepUpGrant, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := stepUpGrantKey(sessionToken, action, app)
	gr, ok := g.m[k]
	if !ok {
		return stepUpGrant{}, false
	}
	if now.After(gr.expires) {
		delete(g.m, k)
		return stepUpGrant{}, false
	}
	if gr.userID != userID {
		return stepUpGrant{}, false
	}
	return gr, true
}

// stepUpProof is what a caller presents to prove it is the account owner.
type stepUpProof struct {
	Method       string `json:"method"`
	Password     string `json:"password,omitempty"`
	Code         string `json:"code,omitempty"`
	RecoveryCode string `json:"recovery_code,omitempty"`
}

func (p *stepUpProof) empty() bool {
	return p == nil || (p.Password == "" && p.Code == "" && p.RecoveryCode == "")
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// equalizeTiming spends a bcrypt compare when there is no real hash, so
// an account without a password answers in the same time as a wrong one.
func equalizeTiming(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("step-up-timing-equalizer"), bcrypt.DefaultCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

var (
	errStepUpInvalid   = errors.New("step-up verification failed")
	errStepUpNeedsCode = errors.New("this account has two-factor authentication: enter the authenticator code as well")
)

type stepUpLockedError struct{ retryAfter time.Duration }

func (e *stepUpLockedError) Error() string { return "too many failed attempts, try again later" }

func stepUpLimiterKeys(r *http.Request, userID string) []string {
	return []string{"user:" + userID, "ip:" + clientIP(r)}
}

func (rt *Router) stepUpAllowed(r *http.Request, userID string) error {
	for _, k := range stepUpLimiterKeys(r, userID) {
		if ok, retry := rt.envProtect.limiter.allow(k); !ok {
			return &stepUpLockedError{retryAfter: retry}
		}
	}
	return nil
}

func (rt *Router) recordStepUp(r *http.Request, userID string, success bool) {
	for _, k := range stepUpLimiterKeys(r, userID) {
		if success {
			rt.envProtect.limiter.recordSuccess(k)
		} else {
			rt.envProtect.limiter.recordFailure(k)
		}
	}
}

// verifyPasswordProof checks the account password and, when the account
// has TOTP enrolled, the authenticator or recovery code as well.
func (rt *Router) verifyPasswordProof(r *http.Request, userID string, p stepUpProof) error {
	if err := rt.stepUpAllowed(r, userID); err != nil {
		rt.auditStepUpOutcome(r.Context(), r, userID, auditActionStepUpLocked, http.StatusTooManyRequests)
		return err
	}
	err := rt.checkPasswordProof(r, userID, p)
	if errors.Is(err, errStepUpNeedsCode) {
		// Counted as an attempt, so the hint cannot become a free password oracle.
		rt.recordStepUp(r, userID, false)
		rt.auditStepUpOutcome(r.Context(), r, userID, auditActionStepUpNeedCode, http.StatusForbidden)
		return err
	}
	var locked *authengine.LockedError
	if errors.As(err, &locked) {
		rt.auditStepUpOutcome(r.Context(), r, userID, auditActionStepUpLocked, http.StatusTooManyRequests)
		return &stepUpLockedError{retryAfter: locked.RetryAfter}
	}
	rt.recordStepUp(r, userID, err == nil)
	rt.auditStepUp(r.Context(), r, userID, err == nil)
	return err
}

func (rt *Router) checkPasswordProof(r *http.Request, userID string, p stepUpProof) error {
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil || user.PasswordHash == nil || *user.PasswordHash == "" {
		equalizeTiming(p.Password)
		return errStepUpInvalid
	}
	if bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(p.Password)) != nil {
		return errStepUpInvalid
	}
	enrolled, err := rt.totpEnabled(r.Context(), userID)
	if err != nil {
		return errStepUpInvalid
	}
	if !enrolled {
		return nil
	}
	if p.Code == "" && p.RecoveryCode == "" {
		return errStepUpNeedsCode
	}
	err = rt.mfaLib.mfa.CheckSecondFactor(r.Context(), userID, p.Code, p.RecoveryCode, r.UserAgent(), clientIP(r))
	var locked *authengine.LockedError
	if errors.As(err, &locked) {
		return err
	}
	if err != nil {
		return errStepUpInvalid
	}
	return nil
}

func (rt *Router) auditStepUp(ctx context.Context, r *http.Request, userID string, ok bool) {
	action, status := auditActionStepUpVerified, http.StatusOK
	if !ok {
		action, status = auditActionStepUpFailed, http.StatusForbidden
	}
	rt.saveEnvAudit(ctx, r, envAuditActor{kind: "session", id: userID}, action, r.URL.Path, status)
}

func (rt *Router) auditStepUpOutcome(ctx context.Context, r *http.Request, userID, action string, status int) {
	rt.saveEnvAudit(ctx, r, envAuditActor{kind: "session", id: userID}, action, r.URL.Path, status)
}

// writeStepUpError maps a verification failure onto a response.
func writeStepUpError(w http.ResponseWriter, err error) {
	var locked *stepUpLockedError
	switch {
	case errors.As(err, &locked):
		secs := int(locked.retryAfter.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeCodedError(w, http.StatusTooManyRequests, "step_up_locked", locked.Error())
	case errors.Is(err, errStepUpNeedsCode):
		writeCodedError(w, http.StatusForbidden, "step_up_code_required", err.Error())
	default:
		writeCodedError(w, http.StatusForbidden, "step_up_invalid", "the password or code is not correct")
	}
}

// sessionUserOnly resolves an ordinary signed-in session. API tokens and
// session links minted from tokens never get a step-up grant.
func (rt *Router) sessionUserOnly(w http.ResponseWriter, r *http.Request) (userID, token string, ok bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeCodedError(w, http.StatusForbidden, "step_up_session_only", "step-up needs a dashboard sign-in, not a session link or API token")
		return "", "", false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeCodedError(w, http.StatusForbidden, "step_up_session_only", "step-up needs a dashboard sign-in, not a session link or API token")
		return "", "", false
	}
	userID, found := rt.currentSessionUserID(r)
	if !found {
		writeCodedError(w, http.StatusForbidden, "step_up_session_only", "step-up needs a dashboard sign-in, not a session link or API token")
		return "", "", false
	}
	return userID, c.Value, true
}

func (rt *Router) userHasPasskeys(ctx context.Context, userID string) bool {
	if rt.mfaLib == nil || rt.mfaLib.mfa == nil || !rt.mfaLib.mfa.PasskeysAvailable() {
		return false
	}
	keys, err := rt.mfaLib.mfa.PasskeyList(ctx, userID)
	return err == nil && len(keys) > 0
}

func (rt *Router) userHasPassword(ctx context.Context, userID string) bool {
	u, err := rt.auth.GetUserByID(ctx, userID)
	return err == nil && u.PasswordHash != nil && *u.PasswordHash != ""
}
