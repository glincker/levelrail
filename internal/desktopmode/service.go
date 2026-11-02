package desktopmode

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Route paths served by Wrap. They sit outside the API router's route table.
const (
	ExchangePath = "/api/desktop/exchange"
	RotatePath   = "/api/desktop/rotate"
	RevokePath   = "/api/desktop/revoke"

	// sessionCookie matches the dashboard's cookie name so the library session is honored as is.
	sessionCookie = "session_token"
)

// Sessions is the slice of the auth library desktop mode needs.
type Sessions interface {
	IssueSessionTTL(ctx context.Context, u authengine.LegacyUser, userAgent, ip string, ttl time.Duration) (string, error)
	Revoke(ctx context.Context, token string)
	RevokeAll(ctx context.Context, legacyUserID string) error
}

// Config wires a Service. Zero limits disable the matching guard.
type Config struct {
	Tokens   *Tokens
	Sessions Sessions
	// Admin returns the root admin of this single-user install.
	Admin func(ctx context.Context) (authengine.LegacyUser, error)
	// TTL is read per exchange so the runtime setting applies live.
	TTL func(ctx context.Context) time.Duration
	// Audit records one row; it must never receive secret values.
	Audit  func(ctx context.Context, e store.AuditEntry)
	Logger *slog.Logger
	// Ports are the dashboard ports (guest and host side) the Host header may carry.
	Ports []string
	// FailuresPerMin bad tokens from one peer start a block of Block; 0 disables the limit.
	FailuresPerMin int
	Block          time.Duration
	Now            func() time.Time
	// AllowRemotePeer lifts the loopback peer requirement; tests only.
	AllowRemotePeer bool
}

// Service is desktop mode: token exchange plus Host and Origin enforcement.
type Service struct {
	cfg   Config
	hosts map[string]bool
	lim   *limiter
	now   func() time.Time
}

// New builds a Service.
func New(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Service{
		cfg: cfg, hosts: AllowedHosts(cfg.Ports), now: now,
		lim: &limiter{cap: cfg.FailuresPerMin, block: cfg.Block, now: now},
	}
}

// Wrap guards every request and serves the three desktop routes; all else goes to next.
func (s *Service) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		if stateChanging(r) && !s.originAllowed(r) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case ExchangePath:
			s.route(w, r, s.exchange)
		case RotatePath:
			s.route(w, r, s.rotate)
		case RevokePath:
			s.route(w, r, s.revoke)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Service) route(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request)) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.AllowRemotePeer && !peerIsLoopback(r) {
		http.Error(w, "loopback only", http.StatusForbidden)
		return
	}
	h(w, r)
}

// authorize checks the bearer token first, in constant time, so a valid token is never
// delayed by someone else's bad attempts. Only failures are limited, per peer.
func (s *Service) authorize(w http.ResponseWriter, r *http.Request, action string) bool {
	presented, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if found && s.cfg.Tokens.Verify(strings.TrimSpace(presented)) {
		return true
	}
	blocked, wait := s.lim.fail(peerHost(r))
	s.audit(r, action+"_failed", http.StatusUnauthorized, map[string]any{"blocked": blocked})
	if blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return false
	}
	http.Error(w, "invalid token", http.StatusUnauthorized)
	return false
}

func (s *Service) exchange(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "exchange") {
		return
	}
	ctx := r.Context()
	admin, err := s.cfg.Admin(ctx)
	if errors.Is(err, ErrNotSingleUser) {
		http.Error(w, "desktop mode needs a single-user install", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, "no admin account yet", http.StatusConflict)
		return
	}
	ttl := s.cfg.TTL(ctx)
	token, err := s.cfg.Sessions.IssueSessionTTL(ctx, admin, r.UserAgent(), r.RemoteAddr, ttl)
	if err != nil {
		s.cfg.Logger.Error("desktop mode: issue session failed", slog.String("error", err.Error()))
		http.Error(w, "session unavailable", http.StatusInternalServerError)
		return
	}
	exp := s.now().Add(ttl)
	// Secure follows the transport: the loopback dashboard is plain http, where a Secure cookie would be dropped.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure only over TLS, see above
		Name: sessionCookie, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
	s.audit(r, "exchange", http.StatusOK, map[string]any{"expires_at": exp.UTC().Format(time.RFC3339)})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"expires_at": exp.UTC().Format(time.RFC3339)})
}

func (s *Service) rotate(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "rotate") {
		return
	}
	if err := s.cfg.Tokens.Rotate(); err != nil {
		s.cfg.Logger.Error("desktop mode: rotate failed", slog.String("error", err.Error()))
		http.Error(w, "rotate failed", http.StatusInternalServerError)
		return
	}
	s.revokeSessions(r.Context())
	s.audit(r, "rotate", http.StatusNoContent, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) revoke(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "revoke") {
		return
	}
	if err := s.cfg.Tokens.Revoke(); err != nil {
		s.cfg.Logger.Error("desktop mode: revoke failed", slog.String("error", err.Error()))
		http.Error(w, "revoke failed", http.StatusInternalServerError)
		return
	}
	s.revokeSessions(r.Context())
	s.audit(r, "revoke", http.StatusNoContent, nil)
	w.WriteHeader(http.StatusNoContent)
}

// revokeSessions ends every session of the admin, whichever process issued it.
func (s *Service) revokeSessions(ctx context.Context) {
	a, err := s.cfg.Admin(ctx)
	if err != nil {
		return
	}
	if err := s.cfg.Sessions.RevokeAll(ctx, a.ID); err != nil {
		s.cfg.Logger.Warn("desktop mode: revoke sessions failed", slog.String("error", err.Error()))
	}
}

func (s *Service) audit(r *http.Request, action string, status int, detail map[string]any) {
	if s.cfg.Audit == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	var d string
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			d = string(b)
		}
	}
	s.cfg.Audit(r.Context(), store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "desktop", ActorName: "desktop app",
		Method: r.Method, Path: r.URL.Path, StatusCode: status, RemoteAddr: r.RemoteAddr,
		CreatedAt: store.FormatAuditTime(s.now()), ClientKind: "system",
		Action: "desktop." + action, Detail: d,
	})
}

// ErrNotSingleUser means the install has several users, which desktop mode does not support.
var ErrNotSingleUser = errors.New("desktopmode: install is not single-user")

// Mode reads APP_DESKTOP_MODE. Unset is off; a value that is not a usual boolean is an
// error so the caller can fail closed instead of silently dropping the checks.
func Mode(getenv func(string) string) (bool, error) {
	switch v := strings.ToLower(strings.TrimSpace(getenv("APP_DESKTOP_MODE"))); v {
	case "":
		return false, nil
	case "1", "true", "yes", "on", "y", "t":
		return true, nil
	case "0", "false", "no", "off", "n", "f":
		return false, nil
	default:
		return false, errors.New("desktopmode: APP_DESKTOP_MODE must be a boolean")
	}
}
