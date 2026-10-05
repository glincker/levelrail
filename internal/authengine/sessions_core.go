package authengine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

const (
	routeSignin = "/email-password/signin"
	routeSignup = "/email-password/signup"
	routeForgot = "/email-password/forgot"
	routeReset  = "/email-password/reset"

	resetLinkMarker   = "/email-password/reset?token="
	setupTokenHeader  = "X-Setup-Token" //nolint:gosec // header name, not a credential
	credentialUserTag = "user:"
	dispatchTimeout   = 15 * time.Second
)

// Sessions serves password login, sessions, session links and password
// reset through the library, translating to and from Levelrail's own ids.
type Sessions struct {
	db     *sql.DB
	hooks  SessionsHooks
	logger *slog.Logger

	mu      sync.RWMutex
	auth    *theauth.TheAuth
	prefix  string
	handler http.Handler
	authn   http.Handler
	events  chan theauth.AuthEvent
	done    chan struct{}
	pumped  chan struct{}
}

type sessionKey struct{}

type sessionSlot struct {
	sess *theauth.Session
	user *theauth.User
}

func newSessions(db *sql.DB, hooks SessionsHooks) *Sessions {
	return &Sessions{
		db:     db,
		hooks:  hooks,
		logger: slog.Default(),
		events: make(chan theauth.AuthEvent, 256),
		done:   make(chan struct{}),
		pumped: make(chan struct{}),
	}
}

// bind attaches the live library instance and starts the audit pump.
func (s *Sessions) bind(a *theauth.TheAuth, prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth = a
	s.prefix = strings.TrimRight(prefix, "/")
	s.handler = a.Handler()
	s.authn = a.Authn()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		slot, _ := r.Context().Value(sessionKey{}).(*sessionSlot)
		if slot == nil {
			return
		}
		slot.sess, _ = theauth.SessionFromContext(r.Context())
		slot.user, _ = theauth.UserFromContext(r.Context())
	}))
	go s.pumpAudit()
}

func (s *Sessions) close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	select {
	case <-s.pumped:
	case <-time.After(dispatchTimeout):
	}
}

// Error is a library failure translated to a status and stable code.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	return fmt.Sprintf("authengine: library returned %d %s: %s", e.Status, e.Code, e.Message)
}

// IsCode reports whether err is an *Error with the given library code.
func IsCode(err error, code string) bool {
	var le *Error
	return errors.As(err, &le) && le.Code == code
}

// Library error codes the host maps to its own messages.
const (
	CodeInvalidCredentials = theauth.CodeInvalidCredentials
	CodeRateLimited        = theauth.CodeRateLimited
	CodeAccountLocked      = theauth.CodeAccountLocked
	CodeWeakPassword       = theauth.CodeWeakPassword
	CodeEmailTaken         = theauth.CodeEmailTaken
	CodeSignupClosed       = theauth.CodeSignupClosed
	CodeSetupTokenInvalid  = theauth.CodeSetupTokenInvalid
	CodeResetInvalid       = theauth.CodePasswordResetInvalid
	CodeResetExpired       = theauth.CodePasswordResetExpired
)

type callOpts struct {
	ip, userAgent, setupToken string
}

type libReply struct {
	status int
	header http.Header
	body   bytes.Buffer
}

func (r *libReply) Header() http.Header         { return r.header }
func (r *libReply) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *libReply) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *libReply) cookieValue(name string) string {
	resp := http.Response{Header: r.header}
	for _, c := range resp.Cookies() {
		if c.Name == name && c.MaxAge >= 0 {
			return c.Value
		}
	}
	return ""
}

func (s *Sessions) call(ctx context.Context, route string, body any, o callOpts) (*libReply, error) {
	s.mu.RLock()
	h, prefix := s.handler, s.prefix
	s.mu.RUnlock()
	if h == nil {
		return nil, errors.New("authengine: sessions engine not bound")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("authengine: encode request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, dispatchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prefix+route, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("authengine: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", o.userAgent)
	if o.setupToken != "" {
		req.Header.Set(setupTokenHeader, o.setupToken)
	}
	req.RemoteAddr = net.JoinHostPort(hostOnly(o.ip), "0")
	rep := &libReply{header: http.Header{}}
	h.ServeHTTP(rep, req)
	if rep.status == 0 {
		rep.status = http.StatusOK
	}
	if rep.status >= http.StatusBadRequest {
		return rep, replyError(rep)
	}
	return rep, nil
}

func hostOnly(ip string) string {
	if h, _, err := net.SplitHostPort(ip); err == nil {
		return h
	}
	if ip == "" {
		return "127.0.0.1"
	}
	return ip
}

func replyError(rep *libReply) *Error {
	e := &Error{Status: rep.status}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(rep.body.Bytes(), &body) == nil {
		e.Code, e.Message = body.Code, body.Message
	}
	if ra := rep.header.Get("Retry-After"); ra != "" {
		var secs int
		if _, err := fmt.Sscanf(ra, "%d", &secs); err == nil && secs > 0 {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return e
}

// SessionInfo is a live full session resolved to the platform's own user id.
type SessionInfo struct {
	LegacyUserID string
	EngineUserID string
	SessionID    string
	ExpiresAt    time.Time
}

// Lookup resolves a session token. It fails closed: a pending second-factor
// session, an unmapped (deleted) user or any lookup error all report false.
func (s *Sessions) Lookup(ctx context.Context, token string) (SessionInfo, bool) {
	if token == "" {
		return SessionInfo{}, false
	}
	s.mu.RLock()
	authn := s.authn
	s.mu.RUnlock()
	if authn == nil {
		return SessionInfo{}, false
	}
	slot := &sessionSlot{}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, sessionKey{}, slot), http.MethodGet, "/", http.NoBody)
	if err != nil {
		return SessionInfo{}, false
	}
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	authn.ServeHTTP(&libReply{header: http.Header{}}, req)
	if slot.sess == nil || slot.user == nil {
		return SessionInfo{}, false
	}
	if lvl := slot.sess.AuthLevel; lvl != "" && lvl != theauth.AuthLevelFull {
		return SessionInfo{}, false
	}
	legacy, err := s.legacyID(ctx, slot.user.ID.String())
	if err != nil || legacy == "" {
		return SessionInfo{}, false
	}
	return SessionInfo{
		LegacyUserID: legacy,
		EngineUserID: slot.user.ID.String(),
		SessionID:    slot.sess.ID.String(),
		ExpiresAt:    slot.sess.ExpiresAt,
	}, true
}

func (s *Sessions) legacyID(ctx context.Context, engineID string) (string, error) {
	var legacy string
	err := s.db.QueryRowContext(ctx, `SELECT legacy_id FROM authengine_user_map WHERE engine_id = ?`, engineID).Scan(&legacy)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("authengine: map engine user: %w", err)
	}
	return legacy, nil
}

func (s *Sessions) engineID(ctx context.Context, legacyID string) (string, error) {
	var engine string
	err := s.db.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, legacyID).Scan(&engine)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("authengine: map legacy user: %w", err)
	}
	return engine, nil
}

// checkCredential re-validates the user a session link or session is bound to.
func (s *Sessions) checkCredential(ctx context.Context, credentialID string) error {
	engine, ok := strings.CutPrefix(credentialID, credentialUserTag)
	if !ok {
		return theauth.ErrCredentialRevoked
	}
	legacy, err := s.legacyID(ctx, engine)
	if err != nil {
		return err
	}
	if legacy == "" {
		return theauth.ErrCredentialRevoked
	}
	return nil
}

func newCookieRequest(ctx context.Context, slot *sessionSlot, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(context.WithValue(ctx, sessionKey{}, slot), http.MethodGet, "/", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("authengine: build lookup request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	return req, nil
}

// Sessions returns the sessions facade.
func (e *Engine) Sessions() *Sessions { return e.sessions }

func bindSessions(s *Sessions, a *theauth.TheAuth, prefix string) {
	s.bind(a, prefix)
}

// Mail returns the relay that carries library email to the host, if any.
func (s *Sessions) Mail() *MailRelay { return s.hooks.Mail }

// EmitPasswordChanged records a password change made through the host.
func (s *Sessions) EmitPasswordChanged(ctx context.Context, engineUserID string) {
	s.mu.RLock()
	a := s.auth
	s.mu.RUnlock()
	if a != nil && engineUserID != "" {
		a.EmitAudit(ctx, "password.changed", theauth.TargetRef{Type: "user", ID: engineUserID}, nil)
	}
}

// RecoverAdmin sets a platform user's password through the library, which
// also revokes the user's sessions and clears login lockouts, in the shared
// database, so a running server sees the change at once.
func RecoverAdmin(ctx context.Context, db *sql.DB, u LegacyUser, newPassword string) error {
	eng, err := New(db, Config{Directory: NewDirectory(db), BaseURL: "http://localhost"})
	if err != nil {
		return fmt.Errorf("authengine: recover admin: %w", err)
	}
	defer eng.Close()
	sess := eng.sessions
	if sess == nil {
		sess = newSessions(db, SessionsHooks{})
	}
	if _, err := sess.SyncUser(ctx, u); err != nil {
		return err
	}
	if err := eng.auth.ResetPasswordAdmin(ctx, u.Email, newPassword); err != nil {
		return fmt.Errorf("authengine: recover admin: reset password: %w", err)
	}
	ident := normEmail(u.Email)
	if _, err := db.ExecContext(ctx, `DELETE FROM theauth_throttle_entries WHERE key LIKE 'login:%' AND substr(key, -length(?) - 1) = '|' || ?`, ident, ident); err != nil {
		return fmt.Errorf("authengine: recover admin: clear login backoff: %w", err)
	}
	return nil
}
