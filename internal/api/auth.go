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
// the project's naming rule bans product-name strings in source, and a
// cookie name doesn't need branding to do its job.
const sessionCookieName = "session_token"

// defaultSessionTTL is the fallback when the control plane isn't given
// an explicit one (Router.sessionTTL, set via WithSessionTTL). The
// project's "no hardcoded thresholds, use env vars" rule is honored at
// cmd/levelrail/main.go, which reads APP_SESSION_TTL and passes it down;
// this constant is only the value used when that env var is unset, the
// same "relative default, env override" shape openStore's data
// directory and loadBrand's file path already use.
const defaultSessionTTL = 24 * time.Hour

// session is one logged-in session, identified by the real user it
// belongs to. Multiple distinct users can each hold any number of live
// sessions; every session grants the same access (see requireAbility).
type session struct {
	userID    string
	expiresAt time.Time
}

// sessionStore is a server-side, in-memory session table: the cookie
// carries only an opaque token, this map is the sole place that token
// resolves to a user ID. In-memory: a restart invalidates every session.
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

func (s *sessionStore) create(userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate session token: %w", err)
	}
	s.mu.Lock()
	s.sessions[token] = session{userID: userID, expiresAt: time.Now().Add(s.ttl)}
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
	return sess.userID, true
}

// get returns the full session record for token, used where a caller
// needs more than lookup's user ID (e.g. handleGetSession also wants
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

// revokeAllExcept deletes every session belonging to userID other than
// keepToken, scoped strictly to that one user: revoking their sessions
// must never touch another user's.
func (s *sessionStore) revokeAllExcept(userID, keepToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if token != keepToken && sess.userID == userID {
			delete(s.sessions, token)
		}
	}
}

// revokeAll deletes every session belonging to userID, unlike
// revokeAllExcept which always spares one token.
func (s *sessionStore) revokeAll(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.userID == userID {
			delete(s.sessions, token)
		}
	}
}

// revokeAll deletes every session belonging to username: the
// reset-password counterpart to revokeAllExcept, since an unauthenticated
// reset has no "current session" to leave alone.
func (s *sessionStore) revokeAll(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.username == username {
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

// randomOpaqueID generates an opaque, URL-safe identifier with the given
// prefix, the same shape store.NewDeployAttemptID/randomTokenID use.
// Kept private to this package: only internal/api mints user/identity IDs.
func randomOpaqueID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate %sid: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

type loginRequest struct {
	Email    string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Email       string `json:"username"`
	DisplayName string `json:"display_name"`
}

// handleLogin verifies an email/password against the users table and,
// on success, sets a session cookie. The wire field stays "username"
// (loginRequest.Email's json tag) for existing callers; email is simply
// what it means now. Same generic "invalid credentials" for unknown
// email or wrong password.
func (rt *Router) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	key := loginLimiterKey(r, req.Email)
	if ok, retryAfter := rt.logins.allow(key); !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	user, err := rt.auth.GetUserByEmail(r.Context(), req.Email)
	if errors.Is(err, store.ErrUserNotFound) {
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		rt.logger.Error("api: login: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.PasswordHash == nil {
		// No password to check against: treat like a wrong password so
		// the response never reveals which sign-in methods an account has.
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		rt.logins.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	rt.logins.recordSuccess(key)

	if err := rt.establishSession(r.Context(), w, *user); err != nil {
		rt.logger.Error("api: login: establish session failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

// establishSession is the one place a session cookie gets created and
// last_login_at gets stamped: handleLogin, handleRegister, and the OAuth
// callback handlers all funnel through this, so every sign-in path
// shares identical session properties by construction.
func (rt *Router) establishSession(ctx context.Context, w http.ResponseWriter, user store.User) error {
	token, err := rt.sessions.create(user.ID)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Best-effort: last_login_at is an observability nicety for the Users
	// settings page, never something a sign-in should fail over.
	if err := rt.auth.UpdateUserLastLogin(ctx, user.ID, time.Now()); err != nil {
		rt.logger.Warn("api: update last_login_at failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
	}

	// Secure is intentional even though this HTTP listener itself speaks
	// plain HTTP (main.go's httpServer): embedded Caddy sits in front
	// doing TLS termination in any real deployment, and a
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
	return nil
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
// authenticated either by a valid session (implicitly AbilityRoot) or a
// bearer token whose stored abilities grant required. A session grants
// full access unconditionally: not because there's only one identity
// (any number of users can exist now), but because there's no per-user
// role model yet (RBAC is Phase 4).
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
	_, ok := rt.currentSessionUserID(r)
	return ok
}

// currentSessionUserID returns the user ID and session token a request's
// session cookie resolves to, if any. handleChangePassword needs both:
// the user ID to load the account being changed, and the token so
// revokeAllExcept can leave this one session alone.
func (rt *Router) currentSessionUserID(r *http.Request) (userID string, ok bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	userID, ok = rt.sessions.lookup(c.Value)
	return userID, ok
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

// handleRegister handles POST /api/v1/auth/register: creates the very
// first user of a genuinely empty instance, nothing else. Every
// subsequent user comes from OAuth (oauth.go) or an authenticated caller
// via POST /api/v1/auth/users: open self-registration is too big a risk
// once this control plane is internet-reachable, with no invite system
// to bound it. Gated at the mutation layer via
// ux_users_single_first_user (migrations/0035), closing the
// two-concurrent-registrations race.
func (rt *Router) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
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

	id, err := randomOpaqueID("user_")
	if err != nil {
		rt.logger.Error("api: register: generate user id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	hashStr := string(hash)
	user := store.User{
		ID:           id,
		Email:        req.Email,
		DisplayName:  req.Email,
		PasswordHash: &hashStr,
		IsFirstUser:  true,
		CreatedAt:    time.Now(),
	}
	if err := rt.auth.CreateUser(r.Context(), user); errors.Is(err, store.ErrFirstUserExists) {
		writeError(w, http.StatusConflict, "an account already exists; sign in instead")
		return
	} else if errors.Is(err, store.ErrUserEmailExists) {
		writeError(w, http.StatusConflict, "an account already exists; sign in instead")
		return
	} else if err != nil {
		rt.logger.Error("api: register: save user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.establishSession(r.Context(), w, user); err != nil {
		rt.logger.Error("api: register: establish session failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

// minPasswordLength is a security floor, not a deployment preference,
// so it stays a constant rather than an env var.
const minPasswordLength = 8

// BootstrapAdmin ensures at least one user exists, creating one from
// username/password if none does yet. No-op once any user exists, so a
// restart with the same env vars never resets a changed password.
// username becomes the new user's email verbatim (no "@" required),
// matching migrations/0035's backfill leniency for a pre-existing
// admin_user row.
func BootstrapAdmin(ctx context.Context, s AuthStore, username, password string) error {
	n, err := s.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("api: bootstrap admin: count existing: %w", err)
	}
	if n > 0 {
		return nil // already bootstrapped
	}
	if username == "" || password == "" {
		return fmt.Errorf("api: bootstrap admin: no user account exists and no username/password were provided")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("api: bootstrap admin: hash password: %w", err)
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		return fmt.Errorf("api: bootstrap admin: generate user id: %w", err)
	}
	hashStr := string(hash)
	user := store.User{
		ID:           id,
		Email:        username,
		DisplayName:  username,
		PasswordHash: &hashStr,
		IsFirstUser:  true,
		CreatedAt:    time.Now(),
	}
	if err := s.CreateUser(ctx, user); err != nil {
		// Losing a concurrent bootstrap race isn't a real failure: "at
		// least one user exists" is what this function promises either way.
		if errors.Is(err, store.ErrFirstUserExists) || errors.Is(err, store.ErrUserEmailExists) {
			return nil
		}
		return fmt.Errorf("api: bootstrap admin: save: %w", err)
	}
	return nil
}
