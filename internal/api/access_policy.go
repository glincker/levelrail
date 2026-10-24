package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/accesspolicy"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const accessPolicyTouchEvery = 30 * time.Second

// AccessPolicyStore is the persistence the session policy needs. *store.DB satisfies it.
type AccessPolicyStore interface {
	ListSessionPolicies(ctx context.Context) ([]store.SessionPolicy, error)
	SessionPoliciesForUser(ctx context.Context, userID string) ([]store.SessionPolicy, error)
	GetSessionPolicy(ctx context.Context, teamID string) (store.SessionPolicy, error)
	SaveSessionPolicy(ctx context.Context, p store.SessionPolicy) error
	DeleteSessionPolicy(ctx context.Context, teamID string) error
	GetSessionActivity(ctx context.Context, sessionID string) (store.SessionActivity, error)
	TouchSessionActivity(ctx context.Context, sessionID, userID string, now time.Time) error
	SetSessionMethod(ctx context.Context, sessionID, userID, method string, now time.Time) error
	TeamMemberRole(ctx context.Context, teamID, userID string) (string, error)
}

// touchCache throttles last-seen writes to one per session per interval.
type touchCache struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (c *touchCache) forget(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.last, sessionID)
}

func (c *touchCache) due(sessionID string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil || len(c.last) > 10000 {
		c.last = map[string]time.Time{}
	}
	if t, ok := c.last[sessionID]; ok && now.Sub(t) < accessPolicyTouchEvery {
		return false
	}
	c.last[sessionID] = now
	return true
}

type policySubject struct {
	userID    string
	sessionID string
	token     string
}

// subjectOf names the user a request acts for: a session's user, or a
// bearer token's owner. Unauthenticated and ownerless requests have none.
func (rt *Router) subjectOf(r *http.Request) policySubject {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if rt.libSessions != nil {
			if info, ok := rt.libSessions.Lookup(r.Context(), c.Value); ok {
				return policySubject{userID: info.LegacyUserID, sessionID: info.SessionID, token: c.Value}
			}
		}
		if pinned, ok := rt.sessions.getPinned(c.Value); ok {
			if rec, err := rt.tokens.GetAPITokenByID(r.Context(), pinned.userID); err == nil {
				return policySubject{userID: rec.OwnerUserID}
			}
		}
	}
	if raw, ok := bearerToken(r); ok && !strings.HasPrefix(r.URL.Path, scimBasePath+"/") {
		if rec, err := rt.lookupBearerToken(r.Context(), raw); err == nil {
			return policySubject{userID: rec.OwnerUserID}
		}
	}
	return policySubject{}
}

// effectiveRules merges the policies of every team the user is in.
func (rt *Router) effectiveRules(ctx context.Context, userID string) (accesspolicy.Rules, error) {
	stored, err := rt.accessPolicy.SessionPoliciesForUser(ctx, userID)
	if err != nil {
		return accesspolicy.Rules{}, err
	}
	return accesspolicy.Merge(toPolicies(stored))
}

func toPolicies(stored []store.SessionPolicy) []accesspolicy.Policy {
	out := make([]accesspolicy.Policy, 0, len(stored))
	for _, p := range stored {
		out = append(out, accesspolicy.Policy{IdleTimeout: p.IdleTimeout, MaxLifetime: p.MaxLifetime, IPAllowlist: p.IPAllowlist, RequireMFA: p.RequireMFA,
			MaxSessions: p.MaxSessions, RejectOnLimit: p.OnLimit == store.SessionOnLimitReject})
	}
	return out
}

func peerTrusted(a netip.Addr) bool {
	if a.IsLoopback() && loopbackTrusted() {
		return true
	}
	return peerIsTrustedProxy(&http.Request{RemoteAddr: net.JoinHostPort(a.String(), "0")})
}

// policyClientAddr is the caller's address, honoring X-Forwarded-For only from trusted proxies.
func policyClientAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return accesspolicy.ClientAddr(peer, strings.Join(r.Header.Values("X-Forwarded-For"), ","), peerTrusted)
}

// mfaExempt lists what a user without a second factor may still reach when
// MFA is required: enough to enroll one, see why, and sign out.
func mfaExempt(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/api/v1/auth/session", p == "/api/v1/auth/logout", p == "/api/v1/brand",
		p == "/api/v1/security/session-policy/effective",
		p == "/api/v1/auth/2fa" || strings.HasPrefix(p, "/api/v1/auth/2fa/"),
		p == "/api/v1/auth/passkeys" || strings.HasPrefix(p, "/api/v1/auth/passkeys/"):
		return true
	}
	return !strings.HasPrefix(p, "/api/")
}

// hasSecondFactor reports whether the user enrolled TOTP or a passkey.
func (rt *Router) hasSecondFactor(ctx context.Context, userID string) (bool, error) {
	totp, err := rt.totpEnabled(ctx, userID)
	if err != nil || totp {
		return totp, err
	}
	if rt.mfaLib == nil || rt.mfaLib.mfa == nil || !rt.mfaLib.mfa.PasskeysAvailable() {
		return false, nil
	}
	keys, err := rt.mfaLib.mfa.PasskeyList(ctx, userID)
	if errors.Is(err, authengine.ErrNotMapped) {
		return false, nil
	}
	return len(keys) > 0, err
}

func writePolicyRefusal(w http.ResponseWriter, status int, v accesspolicy.Verdict, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": string(v)})
}

// accessPolicyMiddleware enforces session policy on every authenticated
// request, before any handler runs. Any failure to evaluate refuses the request.
func (rt *Router) accessPolicyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt.accessPolicy == nil {
			next.ServeHTTP(w, r)
			return
		}
		sub := rt.subjectOf(r)
		if sub.userID == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		rules, err := rt.effectiveRules(ctx, sub.userID)
		if err != nil {
			rt.logger.Error("api: session policy unavailable, refusing", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
			writePolicyRefusal(w, http.StatusServiceUnavailable, "session_policy_unavailable", "the session policy could not be evaluated")
			return
		}
		if !rules.IPAllowed(policyClientAddr(r)) {
			writePolicyRefusal(w, http.StatusForbidden, accesspolicy.AddressNotAllowed, "your network address is not allowed by the session policy")
			return
		}
		if sub.sessionID != "" && !rt.checkSessionRules(w, r, sub, rules) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rt *Router) checkSessionRules(w http.ResponseWriter, r *http.Request, sub policySubject, rules accesspolicy.Rules) bool {
	ctx := r.Context()
	now := time.Now()
	act, err := rt.accessPolicy.GetSessionActivity(ctx, sub.sessionID)
	if err != nil {
		rt.logger.Error("api: session activity unavailable, refusing", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
		writePolicyRefusal(w, http.StatusServiceUnavailable, "session_policy_unavailable", "the session policy could not be evaluated")
		return false
	}
	if v := rules.SessionAge(now, act.CreatedAt, act.LastSeenAt, act.Seen); v != accesspolicy.Allowed {
		rt.sessions.revoke(sub.token)
		setSessionCookie(w, r, "", time.Time{})
		writePolicyRefusal(w, http.StatusUnauthorized, v, "your session ended under the session policy: sign in again")
		return false
	}
	if rules.IdleTimeout > 0 && rt.policyTouches.due(sub.sessionID, now) {
		if err := rt.accessPolicy.TouchSessionActivity(ctx, sub.sessionID, sub.userID, now); err != nil {
			rt.logger.Warn("api: touch session activity failed", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
		}
	}
	if !rt.enforceSessionCap(w, r, sub, rules, now) {
		return false
	}
	if !rules.RequireMFA || act.Method == signInMethodSAML || mfaExempt(r) {
		return true
	}
	ok, err := rt.hasSecondFactor(ctx, sub.userID)
	if err != nil {
		rt.logger.Error("api: second factor lookup failed, refusing", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
		writePolicyRefusal(w, http.StatusServiceUnavailable, "session_policy_unavailable", "the session policy could not be evaluated")
		return false
	}
	if !ok {
		writePolicyRefusal(w, http.StatusForbidden, accesspolicy.MFAEnrollmentNeeds, "the session policy requires two-factor sign-in: add an authenticator app or a passkey under Settings, Account")
		return false
	}
	return true
}

// enforceSessionCap applies per-team session caps, which the library's single global cap cannot express.
// Failing to list sessions refuses the request.
func (rt *Router) enforceSessionCap(w http.ResponseWriter, r *http.Request, sub policySubject, rules accesspolicy.Rules, now time.Time) bool {
	if rules.MaxSessions <= 0 || rt.libSessions == nil || !rt.policyCapChecks.due(sub.sessionID, now) {
		return true
	}
	ctx := r.Context()
	list, err := rt.libSessions.ListUserSessions(ctx, sub.userID, sub.token)
	if err != nil {
		rt.logger.Error("api: session cap check failed, refusing", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
		rt.policyCapChecks.forget(sub.sessionID)
		writePolicyRefusal(w, http.StatusServiceUnavailable, "session_policy_unavailable", "the session policy could not be evaluated")
		return false
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
	rank := -1
	for i := range list {
		if list[i].ID == sub.sessionID {
			rank = i
		}
	}
	if rank < 0 {
		return true
	}
	verdict := rules.OverCap(rank, len(list))
	if !rules.RejectOnLimit {
		for i := 0; i < len(list)-rules.MaxSessions; i++ {
			if list[i].ID == sub.sessionID {
				continue
			}
			if _, err := rt.libSessions.RevokeUserSession(ctx, sub.userID, list[i].ID); err != nil {
				rt.logger.Warn("api: evict session over cap failed", slog.String("user_id", sub.userID), slog.String("error", err.Error()))
			}
		}
	}
	switch verdict {
	case accesspolicy.Allowed:
		return true
	case accesspolicy.SessionEvicted:
		rt.sessions.revoke(sub.token)
		setSessionCookie(w, r, "", time.Time{})
		writePolicyRefusal(w, http.StatusUnauthorized, verdict, "your session ended because a newer sign-in replaced it under the session limit: sign in again")
	default:
		rt.sessions.revoke(sub.token)
		setSessionCookie(w, r, "", time.Time{})
		writePolicyRefusal(w, http.StatusConflict, verdict, "the session limit for your account is reached: end another session, then sign in again")
	}
	return false
}
