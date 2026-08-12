package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// sessionCookieName is deliberately generic, not a brand-derived string:
// CLAUDE.md section 3 bans product-name strings in source, and a cookie
// name doesn't need branding to do its job.
const sessionCookieName = "session_token"

const sessionTTL = 24 * time.Hour

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
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]session)}
}

func (s *sessionStore) create(username string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate session token: %w", err)
	}
	s.mu.Lock()
	s.sessions[token] = session{username: username, expiresAt: time.Now().Add(sessionTTL)}
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

func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
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

	admin, err := rt.auth.GetAdminUser(r.Context())
	if errors.Is(err, store.ErrAdminNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		rt.logger.Error("api: login: load admin user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if admin.Username != req.Username {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

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
		Expires:  time.Now().Add(sessionTTL),
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
// wrapped handlers never need to check auth themselves.
func (rt *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if _, ok := rt.sessions.lookup(c.Value); !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r)
	}
}

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
