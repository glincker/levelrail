package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// sessionCookieName is deliberately generic, not a brand-derived string:
// CLAUDE.md section 3 bans product-name strings in source, and a cookie
// name doesn't need branding to do its job.
const sessionCookieName = "session_token"

// defaultSessionTTL is the fallback when the control plane isn't given
// an explicit one (Router.sessionTTL, set via WithSessionTTL). CLAUDE.md
// 7's "no hardcoded thresholds, use env vars" rule is honored at
// cmd/levelrail/main.go, which reads APP_SESSION_TTL and passes it down;
// this constant is only the value used when that env var is unset, the
// same "relative default, env override" shape openStore's data
// directory and loadBrand's file path already use.
const defaultSessionTTL = 24 * time.Hour

// session is one logged-in admin session.
type session struct {
	username  string
	expiresAt time.Time
}

// sessionStore is a server-side, in-memory session table: the cookie
// carries only an opaque token, this map is the sole place that token
// resolves to a username. CLAUDE.md 6 Phase 1 scopes auth to "single
// admin user, session auth", explicitly not JWT/OAuth, so nothing more
// elaborate than this is built here. In-memory is an accepted tradeoff
// for this pass: a restart invalidates every session, which just means
// logging in again, not a correctness problem for a single-node control
// plane.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	ttl      time.Duration
}

// newSessionStore builds a sessionStore. ttl <= 0 falls back to
// defaultSessionTTL, so callers that construct one directly (existing
// tests, primarily) don't need to know about the default themselves.
func newSessionStore(ttl time.Duration) *sessionStore {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &sessionStore{sessions: make(map[string]session), ttl: ttl}
}

func (s *sessionStore) create(username string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate session token: %w", err)
	}
	s.mu.Lock()
	s.sessions[token] = session{username: username, expiresAt: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return token, nil
}

func (s *sessionStore) lookup(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(sess.expiresAt) {
		delete(s.sessions, token)
		return "", false
	}
	return sess.username, true
}

// get returns the full session record for token, used where a caller
// needs more than lookup's username (e.g. handleGetSession also wants
// expiresAt). Same liveness/expiry handling as lookup.
func (s *sessionStore) get(token string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return session{}, false
	}
	if time.Now().After(sess.expiresAt) {
		delete(s.sessions, token)
		return session{}, false
	}
	return sess, true
}

func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// revokeAllExcept deletes every session belonging to username other than
// keepToken. Used by handleChangePassword: rotating every other session
// on a password change (docs-local/research/competitor-onboarding-auth-
// ux.md finding 6, "rotate/invalidate all sessions on password change")
// without logging the caller themselves out of the request that just
// made the change.
func (s *sessionStore) revokeAllExcept(username, keepToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if token != keepToken && sess.username == username {
			delete(s.sessions, token)
		}
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Username string `json:"username"`
}

// handleLogin verifies a username/password against the single admin
// account and, on success, sets a session cookie. Deliberately returns
// the same generic "invalid credentials" message whether the username
// doesn't match, the admin account doesn't exist yet, or the password is
// wrong, so the response never tells an attacker which case they hit.
//
// Rate limited per (client IP, attempted username) via rt.logins: a
// locked-out key is rejected before any bcrypt comparison even runs
// (bcrypt is deliberately slow, and running it anyway on a
// known-locked-out request would just burn CPU on a request that was
// always going to fail), and every failure path below (unknown admin,
// wrong username, wrong password) counts against the limiter, not just
// wrong-password specifically, so probing for a valid username costs
// the same as guessing its password.
func (rt *Router) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	key := loginLimiterKey(r, req.Username)
	if ok, retryAfter := rt.logins.allow(key); !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	admin, err := rt.auth.GetAdminUser(r.Context())
	if errors.Is(err, store.ErrAdminNotFound) {
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		rt.logger.Error("api: login: load admin user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if admin.Username != req.Username {
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	rt.logins.recordSuccess(key)

	token, err := rt.sessions.create(admin.Username)
	if err != nil {
		rt.logger.Error("api: login: create session failed", slog.String("error", err.Error()), slog.String("username", admin.Username))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Secure is intentional even though this HTTP listener itself speaks
	// plain HTTP (main.go's httpServer): CLAUDE.md 4.5 puts embedded Caddy
	// in front doing TLS termination in any real deployment, and a
	// session cookie is exactly the kind of value that must never be
	// sent back over a plain connection. Local development against this
	// listener directly (no Caddy in front yet, TASKS.md 1.6) needs to go
	// through something that terminates TLS, e.g. a local reverse proxy,
	// for the cookie to round-trip; that's a known dev-workflow gap, not
	// a reason to weaken the cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(rt.sessions.ttl),
	})
	writeJSON(w, http.StatusOK, loginResponse{Username: admin.Username})
}

// handleLogout revokes the session named by the request's cookie, if
// any, and clears the cookie client-side. Always succeeds: logging out
// with no session, or an already-expired one, isn't an error case.
func (rt *Router) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		rt.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// requireAuth wraps a handler so it only runs for a request carrying a
// valid, unexpired session cookie; on failure it writes 401 itself, so
// wrapped handlers never need to check auth themselves. Session-only,
// deliberately: used for routes (token management, logout) where a
// bearer token must never be able to act on another token's behalf.
func (rt *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rt.hasValidSession(r) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r)
	}
}

// requireAbility wraps a handler so it only runs for a request
// authenticated either by a valid session (treated as implicitly
// AbilityRoot: CLAUDE.md 6 Phase 1 has exactly one human identity, the
// admin, so there is nothing narrower to scope a session to) or a
// bearer token whose stored abilities grant required. This is what lets
// an MCP-issued or CLI-issued token be provably restricted to, say,
// AbilityRead: hasAbility is checked on every call against the token's
// row read fresh from the store, never a cached decision, matching
// Coolify's own "re-evaluate policies fresh" rule
// (docs-local/research/competitor-onboarding-auth-ux.md).
func (rt *Router) requireAbility(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt.hasValidSession(r) {
			next(w, r)
			return
		}

		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(token))
		if errors.Is(err, store.ErrAPITokenNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if err != nil {
			rt.logger.Error("api: token lookup failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if rec.RevokedAt != nil {
			writeError(w, http.StatusUnauthorized, "token revoked")
			return
		}
		if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
			writeError(w, http.StatusUnauthorized, "token expired")
			return
		}
		if !hasAbility(rec.Abilities, required) {
			writeError(w, http.StatusForbidden, "token lacks the required ability")
			return
		}

		// Best-effort: an observability nicety, must never block or fail
		// the request it's authenticating.
		if terr := rt.tokens.TouchAPITokenLastUsed(r.Context(), rec.ID); terr != nil {
			rt.logger.Warn("api: touch token last_used_at failed", slog.String("error", terr.Error()), slog.String("token_id", rec.ID))
		}

		next(w, r)
	}
}

// hasValidSession reports whether r carries a session cookie that
// resolves to a live session. Factored out of requireAuth so
// requireAbility can check it without duplicating the cookie-lookup
// dance.
func (rt *Router) hasValidSession(r *http.Request) bool {
	_, ok := rt.currentSessionUsername(r)
	return ok
}

// currentSessionUsername returns the username and session token a
// request's session cookie resolves to, if any. handleChangePassword
// needs both: the username to re-verify the current password against,
// and the token so revokeAllExcept can leave this one session alone.
func (rt *Router) currentSessionUsername(r *http.Request) (username string, ok bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	username, ok = rt.sessions.lookup(c.Value)
	return username, ok
}

// bearerToken extracts the token from a "Bearer <token>" Authorization
// header, case-insensitively on the scheme per RFC 6750, empty/absent
// header reported as not-present rather than an error: the caller
// (requireAbility) treats "no token" and "malformed header" identically,
// both mean "not authenticated this way, try the next thing" (there is
// no next thing here, but the shape matches how a future auth method
// would compose).
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// hashToken returns the hex-encoded SHA-256 digest of a plaintext API
// token, the only form ever persisted (store.APIToken.TokenHash).
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// handleRegister handles POST /api/v1/auth/register: the interactive,
// UI-driven counterpart to BootstrapAdmin's env-var path. Coexists with
// it rather than replacing it (a scripted/CI install still sets
// APP_ADMIN_USERNAME/APP_ADMIN_PASSWORD and never touches this route);
// this exists for an operator who starts the binary with neither env
// var set and expects a real first-run screen, per
// docs-local/research/competitor-onboarding-auth-ux.md's synthesis
// (finding 1: reuse one registration form for the first-run case, gated
// server-side on "does an admin row exist," never a hardcoded default
// credential).
//
// Gated at the mutation layer, not just by an earlier caller-side check:
// this calls store.CreateAdminUser, a plain INSERT with no ON CONFLICT
// clause, so admin_user's own PRIMARY KEY constraint (id = 1) is what
// actually decides between two concurrent registration attempts, not a
// GetAdminUser check performed moments earlier that both requests could
// otherwise pass (a read-then-write race a route-level check alone
// cannot close, since both reads can observe "not found" before either
// write lands). Once an admin exists, every subsequent call 409s,
// permanently: there is no "invite a second user" story in Phase 1
// ("single admin user... no teams, no RBAC yet"), and re-registering is
// not how a forgotten password gets reset (see the recovery-path task).
func (rt *Router) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: register: hash password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.auth.CreateAdminUser(r.Context(), req.Username, string(hash)); errors.Is(err, store.ErrAdminAlreadyExists) {
		writeError(w, http.StatusConflict, "an admin account already exists")
		return
	} else if err != nil {
		rt.logger.Error("api: register: save admin failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := rt.sessions.create(req.Username)
	if err != nil {
		rt.logger.Error("api: register: create session failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(rt.sessions.ttl),
	})
	writeJSON(w, http.StatusCreated, loginResponse{Username: req.Username})
}

// minPasswordLength matches CLAUDE.md 7's "no hardcoded thresholds"
// spirit as closely as a fixed minimum reasonably can: unlike a
// tunable operational limit (a timeout, a retry count), a minimum
// password length is a security floor, not a deployment-specific
// preference, so it stays a constant rather than an env var.
const minPasswordLength = 8

// BootstrapAdmin ensures a single admin account exists, creating one
// from username/password if none does yet. It is a no-op once an admin
// exists: cmd/levelrail/main.go calls this on every startup, and only
// the very first call (empty admin_user table) actually writes anything,
// so re-running it with the same env vars after a restart never resets a
// password the operator has since changed.
func BootstrapAdmin(ctx context.Context, s AuthStore, username, password string) error {
	_, err := s.GetAdminUser(ctx)
	if err == nil {
		return nil // already bootstrapped
	}
	if !errors.Is(err, store.ErrAdminNotFound) {
		return fmt.Errorf("api: bootstrap admin: check existing: %w", err)
	}
	if username == "" || password == "" {
		return fmt.Errorf("api: bootstrap admin: no admin account exists and no username/password were provided")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("api: bootstrap admin: hash password: %w", err)
	}
	if err := s.UpsertAdminUser(ctx, username, string(hash)); err != nil {
		return fmt.Errorf("api: bootstrap admin: save: %w", err)
	}
	return nil
}
