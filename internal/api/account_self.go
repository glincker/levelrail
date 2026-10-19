package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// accountSelfStore is the slice of the user store the self-service account
// routes add; *store.DB satisfies it.
type accountSelfStore interface {
	GetUserPreferences(ctx context.Context, userID string) (store.UserPreferences, error)
	SetUserPreferences(ctx context.Context, p store.UserPreferences) error
	DeleteOAuthIdentityForUser(ctx context.Context, userID, provider string) error
	CreateEmailChangeRequest(ctx context.Context, req store.EmailChangeRequest) error
	ConsumeEmailChange(ctx context.Context, tokenHash, userID string, now time.Time) (string, error)
}

// accountSelfState holds the in-memory export jobs and per-user cooldowns; the zero value is ready to use.
type accountSelfState struct {
	mu         sync.Mutex
	exports    map[string]*accountExport
	lastExport map[string]time.Time
	emailAsks  map[string][]time.Time
}

// sweep drops expired export jobs and spent cooldown entries; the janitor ticker calls it.
func (s *accountSelfState) sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.exports {
		if now.After(e.expires) {
			delete(s.exports, k)
		}
	}
	for k, at := range s.lastExport {
		if now.Sub(at) >= exportCooldown {
			delete(s.lastExport, k)
		}
	}
	for k, asks := range s.emailAsks {
		if recent := recentWithin(asks, now, emailChangeWindow); len(recent) == 0 {
			delete(s.emailAsks, k)
		} else {
			s.emailAsks[k] = recent
		}
	}
}

func recentWithin(times []time.Time, now time.Time, window time.Duration) []time.Time {
	out := times[:0:0]
	for _, t := range times {
		if now.Sub(t) < window {
			out = append(out, t)
		}
	}
	return out
}

// takeEmailChange counts one email change request for userID, false once the hourly limit is spent.
func (s *accountSelfState) takeEmailChange(userID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emailAsks == nil {
		s.emailAsks = map[string][]time.Time{}
	}
	recent := recentWithin(s.emailAsks[userID], now, emailChangeWindow)
	if len(recent) >= emailChangeLimit {
		s.emailAsks[userID] = recent
		return false
	}
	s.emailAsks[userID] = append(recent, now)
	return true
}

// takeExport reserves the user's export slot, false while the cooldown runs.
func (s *accountSelfState) takeExport(userID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastExport == nil {
		s.lastExport = map[string]time.Time{}
	}
	if at, ok := s.lastExport[userID]; ok && now.Sub(at) < exportCooldown {
		return false
	}
	s.lastExport[userID] = now
	return true
}

func (rt *Router) accountDB() accountSelfStore {
	for _, c := range []any{rt.policies, rt.apps, rt.tokens, rt.auditLog} {
		if s, ok := c.(accountSelfStore); ok {
			return s
		}
	}
	return nil
}

// accountMutationMu serializes the checks and writes that must not interleave (last sign-in method, last root).
var accountMutationMu sync.Mutex

// accountCaller resolves the signed-in account behind a self-service call. A
// session is the account itself; a token must belong to a user and hold need.
// The account is never read from the request, only from the credential.
func (rt *Router) accountCaller(w http.ResponseWriter, r *http.Request, need string) (userID string, viaSession, ok bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeError(w, http.StatusForbidden, "a session link has no account of its own")
		return "", false, false
	}
	if id, found := rt.currentSessionUserID(r); found {
		return id, true, true
	}
	rec, found := r.Context().Value(tokenIdentityKey{}).(*store.APIToken)
	if !found {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return "", false, false
	}
	if rec.OwnerUserID == "" {
		writeError(w, http.StatusForbidden, "this token is not tied to a user account")
		return "", false, false
	}
	if !hasAbility(rec.Abilities, need) {
		writeError(w, http.StatusForbidden, "token lacks the required ability (needs "+need+")")
		return "", false, false
	}
	if rec.AgentName != "" {
		writeError(w, http.StatusForbidden, "agent tokens cannot manage an account")
		return "", false, false
	}
	attached, err := rt.policies.ListPoliciesForPrincipal(r.Context(), store.PrincipalTypeToken, rec.ID)
	if err != nil {
		rt.internalError(w, "api: account caller: list token policies failed", err)
		return "", false, false
	}
	if len(attached) > 0 {
		writeError(w, http.StatusForbidden, "a scoped token cannot manage an account")
		return "", false, false
	}
	return rec.OwnerUserID, false, true
}

// accountSessionCaller is accountCaller restricted to a real dashboard session.
func (rt *Router) accountSessionCaller(w http.ResponseWriter, r *http.Request) (string, bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeError(w, http.StatusForbidden, "a session link has no account of its own")
		return "", false
	}
	id, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusForbidden, "this needs a signed-in dashboard session")
		return "", false
	}
	return id, true
}

// Stable event names stored in audit_log.action for the caller's own feed.
const (
	accountActionPrefix          = "account."
	accountEventSignIn           = "account.sign_in"
	accountEventPasswordChanged  = "account.password_changed"
	accountEvent2FAEnabled       = "account.2fa_enabled"
	accountEvent2FADisabled      = "account.2fa_disabled"
	accountEventRecoveryCodes    = "account.recovery_codes_regenerated"
	accountEventPasskeyAdded     = "account.passkey_added"
	accountEventPasskeyRemoved   = "account.passkey_removed"
	accountEventPasskeyRenamed   = "account.passkey_renamed"
	accountEventSessionsRevoked  = "account.sessions_revoked"
	accountEventSessionRevoked   = "account.session_revoked"
	accountEventDeviceRemoved    = "account.trusted_device_removed"
	accountEventProfileUpdated   = "account.profile_updated"
	accountEventTokenCreated     = "account.token_created"
	accountEventPrefsUpdated     = "account.preferences_updated"
	accountEventExportRequested  = "account.export_requested"
	accountEventExportDownloaded = "account.export_downloaded"
	accountEventProviderUnlinked = "account.provider_unlinked"
	accountEventEmailRequested   = "account.email_change_requested"
	accountEventEmailChanged     = "account.email_changed"
	accountEventDeleted          = "account.deleted"
)

// accountEventRoute maps a successful request onto an account event, so the
// handlers behind it stay untouched.
type accountEventRoute struct {
	method string
	path   string
	prefix bool
	action string
}

var accountEventRoutes = []accountEventRoute{
	{http.MethodPut, "/api/v1/auth/password", false, accountEventPasswordChanged},
	{http.MethodPost, "/api/v1/auth/2fa/confirm", false, accountEvent2FAEnabled},
	{http.MethodPost, "/api/v1/auth/2fa/disable", false, accountEvent2FADisabled},
	{http.MethodPost, "/api/v1/auth/2fa/recovery-codes/regenerate", false, accountEventRecoveryCodes},
	{http.MethodPost, "/api/v1/auth/passkeys/register/finish", false, accountEventPasskeyAdded},
	{http.MethodDelete, "/api/v1/auth/passkeys/", true, accountEventPasskeyRemoved},
	{http.MethodPatch, "/api/v1/auth/passkeys/", true, accountEventPasskeyRenamed},
	{http.MethodPost, "/api/v1/auth/sessions/revoke-others", false, accountEventSessionsRevoked},
	{http.MethodPost, "/api/v1/security/sessions/revoke-others", false, accountEventSessionsRevoked},
	{http.MethodDelete, "/api/v1/security/sessions/", true, accountEventSessionRevoked},
	{http.MethodDelete, "/api/v1/auth/trusted-devices/", true, accountEventDeviceRemoved},
	{http.MethodPatch, "/api/v1/auth/profile", false, accountEventProfileUpdated},
	{http.MethodPost, "/api/v1/auth/tokens", false, accountEventTokenCreated},
}

func matchAccountEventRoute(method, path string) (string, bool) {
	for _, e := range accountEventRoutes {
		if e.method != method {
			continue
		}
		if (!e.prefix && path == e.path) || (e.prefix && strings.HasPrefix(path, e.path) && len(path) > len(e.path)) {
			return e.action, true
		}
	}
	return "", false
}

// accountEventMiddleware records one feed row per successful account-changing
// request. The actor is resolved before the handler runs, since password and
// session changes can end the very session that made them.
func (rt *Router) accountEventMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action, tracked := matchAccountEventRoute(r.Method, r.URL.Path)
		if !tracked {
			next.ServeHTTP(w, r)
			return
		}
		userID, ok := rt.currentSessionUserID(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status >= 200 && rec.status < 300 {
			rt.recordAccountEvent(context.WithoutCancel(r.Context()), r, userID, action, rec.status)
		}
	})
}

// recordAccountEvent writes one row of the user's own activity feed. Best
// effort: a failed write is logged and never fails the request.
func (rt *Router) recordAccountEvent(ctx context.Context, r *http.Request, userID, action string, status int) {
	rt.recordAccountEntry(ctx, r, auditActorSession, userID, rt.auditActorName(ctx, auditActorSession, userID, ""), action, status)
}

// recordAccountAction records an event for whichever credential made the call.
func (rt *Router) recordAccountAction(r *http.Request, userID string, viaSession bool, action string, status int) {
	ctx := context.WithoutCancel(r.Context())
	if viaSession {
		rt.recordAccountEvent(ctx, r, userID, action, status)
		return
	}
	if rec, ok := r.Context().Value(tokenIdentityKey{}).(*store.APIToken); ok {
		rt.recordAccountEntry(ctx, r, "token", rec.ID, rec.Name, action, status)
	}
}

func (rt *Router) recordAccountEntry(ctx context.Context, r *http.Request, actorType, actorID, actorName, action string, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: account event id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save account event failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
