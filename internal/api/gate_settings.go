package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

// Env vars for the built-in auth gate provider.
const (
	EnvGateSessionTTL     = "APP_GATE_SESSION_TTL"
	EnvGateTokenTTL       = "APP_GATE_TOKEN_TTL" //nolint:gosec // env var name, not a credential
	EnvGateEventRetention = "APP_GATE_EVENT_RETENTION"
	EnvGateAllowHTTP      = "APP_GATE_ALLOW_HTTP"
)

const (
	defaultGateSessionTTL     = 12 * time.Hour
	defaultGateTokenTTL       = time.Minute
	maxGateTokenTTL           = 10 * time.Minute
	defaultGateEventRetention = 30 * 24 * time.Hour
	gateAllowFlushInterval    = 15 * time.Second
)

// GateSettings tunes the built-in auth gate provider.
type GateSettings struct {
	SessionTTL     time.Duration
	TokenTTL       time.Duration
	EventRetention time.Duration
	// AllowHTTP lets gated apps served over plain http sign in; the session
	// cookie then cannot be Secure.
	AllowHTTP bool
}

// DefaultGateSettings returns the settings used when no env var is set.
func DefaultGateSettings() GateSettings {
	return GateSettings{SessionTTL: defaultGateSessionTTL, TokenTTL: defaultGateTokenTTL, EventRetention: defaultGateEventRetention}
}

// parseAuthGateDuration takes a Go duration or whole days ("30d").
func parseAuthGateDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("%q is not a positive number of days", raw)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q is not a positive duration", raw)
	}
	return d, nil
}

// GateSettingsFromEnv reads the APP_GATE_* env vars. On a bad value it
// keeps that setting's default and reports the first error.
func GateSettingsFromEnv(getenv func(string) string) (GateSettings, error) {
	s := DefaultGateSettings()
	var firstErr error
	read := func(name string, dst *time.Duration) {
		raw := getenv(name)
		if strings.TrimSpace(raw) == "" {
			return
		}
		d, err := parseAuthGateDuration(raw)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", name, err)
			}
			return
		}
		*dst = d
	}
	read(EnvGateSessionTTL, &s.SessionTTL)
	read(EnvGateTokenTTL, &s.TokenTTL)
	read(EnvGateEventRetention, &s.EventRetention)
	if s.TokenTTL > maxGateTokenTTL {
		s.TokenTTL = maxGateTokenTTL
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(getenv(EnvGateAllowHTTP))); err == nil {
		s.AllowHTTP = v
	}
	return s, firstErr
}

// GateStore is the store surface the built-in auth gate provider needs.
type GateStore interface {
	GetDomainAuthGate(ctx context.Context, domain string) (store.DomainAuthGate, bool, error)
	ListGateRules(ctx context.Context, domain string) ([]store.GateRule, error)
	ReplaceGateRules(ctx context.Context, domain string, rules []store.GateRule) error
	CreateGateToken(ctx context.Context, t store.GateToken) error
	ConsumeGateToken(ctx context.Context, tokenHash string) (store.GateToken, bool, error)
	CreateGateSession(ctx context.Context, s store.GateSession) error
	GetGateSession(ctx context.Context, tokenHash string) (store.GateSession, bool, error)
	RevokeGateSessions(ctx context.Context, domain string) (int64, error)
	CountLiveGateSessions(ctx context.Context, domain string) (int, error)
	DashboardSessionLive(ctx context.Context, sessionID string) (bool, error)
	DomainEnvironmentID(ctx context.Context, domain string) (string, error)
	RecordAuthGateEvent(ctx context.Context, e store.GateEvent) error
	ListAuthGateEvents(ctx context.Context, domain string, since time.Time, limit int) ([]store.GateEvent, error)
	PruneGateData(ctx context.Context, cutoff time.Time) (int64, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	GetUserByEmail(ctx context.Context, email string) (*store.User, error)
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
	UserRoleID(ctx context.Context, userID string) (string, error)
	UserVisibility(ctx context.Context, userID string) (store.Visibility, error)
	GetOrganization(ctx context.Context, id string) (store.Organization, error)
	ListOrganizations(ctx context.Context) ([]store.Organization, error)
	GetRole(ctx context.Context, id string) (store.Role, error)
	ListRoles(ctx context.Context) ([]store.Role, error)
}

// SetGate enables the built-in auth gate provider. An empty secret leaves
// the check endpoint refusing every request.
func (rt *Router) SetGate(secret string, settings GateSettings) {
	rt.gateSecret = secret
	rt.gateSettings = settings
}

func (rt *Router) effectiveGateSettings() GateSettings {
	s := rt.gateSettings
	d := DefaultGateSettings()
	if s.SessionTTL <= 0 {
		s.SessionTTL = d.SessionTTL
	}
	if s.TokenTTL <= 0 {
		s.TokenTTL = d.TokenTTL
	}
	if s.EventRetention <= 0 {
		s.EventRetention = d.EventRetention
	}
	return s
}

// RunGateSweeper prunes old gate events and expired tokens and sessions on
// interval until ctx is done.
func (rt *Router) RunGateSweeper(ctx context.Context, interval time.Duration) error {
	if rt.gate == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	flush := time.NewTicker(gateAllowFlushInterval)
	defer flush.Stop()
	for {
		select {
		case <-ctx.Done():
			rt.flushGateEvents(context.WithoutCancel(ctx))
			return ctx.Err()
		case <-flush.C:
			rt.flushGateEvents(ctx)
		case <-ticker.C:
			cutoff := time.Now().Add(-rt.effectiveGateSettings().EventRetention)
			if n, err := rt.gate.PruneGateData(ctx, cutoff); err != nil {
				rt.logger.Warn("api: auth gate sweep tick failed", slog.String("error", err.Error()))
			} else if n > 0 {
				rt.logger.Info("api: swept auth gate data", slog.Int64("deleted", n))
			}
		}
	}
}

// newGateSecretToken returns a random URL-safe token and its storage hash.
func newGateSecretToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("api: generate gate token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashGateToken(token), nil
}

func hashGateToken(token string) string {
	return tokenhash.Sum(token)
}

// validGateTokenShape rejects anything that is not a token we could have
// minted, before it reaches the store.
func validGateTokenShape(s string) bool {
	if len(s) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil
}
