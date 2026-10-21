package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gitConnectionStore persists provider connection health; *store.DB satisfies it.
type gitConnectionStore interface {
	SetGitConnectionStatus(ctx context.Context, s store.GitConnectionStatus) error
	GetGitConnectionStatus(ctx context.Context, provider string) (store.GitConnectionStatus, bool, error)
	DeleteGitConnectionStatus(ctx context.Context, provider string) error
}

const (
	codeNeedsReauthorize = store.GitConnNeedsReauthorize
	codeRevoked          = store.GitConnRevoked
)

// gitConnError is a provider connection that needs the operator: the grant
// was revoked or expired, or the installation is gone. Handlers answer it
// with a 409 and a stable code instead of a generic 500.
type gitConnError struct {
	Provider string
	// Key identifies the connection the state belongs to: a connection id,
	// or the provider name for GitHub.
	Key       string
	Status    string
	Detail    string
	NextCheck time.Time
	Err       error
}

func (e *gitConnError) Error() string {
	return fmt.Sprintf("%s connection %s: %s: %v", e.Provider, e.Status, e.Detail, e.Err)
}

func (e *gitConnError) Unwrap() error { return e.Err }

// classifyGitConnErr maps a provider failure on the credential path (token
// refresh, installation token mint, repository listing) to a connection state.
func classifyGitConnErr(provider, key string, err error, now time.Time, removedOn404 bool) *gitConnError {
	if rl, ok := gitprovider.AsRateLimit(err); ok {
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnRateLimited, Detail: "the provider is rate limiting this control plane", NextCheck: rl.NextCheck, Err: err}
	}
	var apiErr *gitprovider.APIError
	if !errors.As(err, &apiErr) {
		return nil
	}
	switch {
	case apiErr.RateLimited:
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnRateLimited, Detail: "the provider is rate limiting this control plane", NextCheck: now.Add(time.Minute), Err: err}
	case apiErr.StatusCode == http.StatusUnauthorized:
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnNeedsReauthorize, Detail: "the provider rejected the stored credential", Err: err}
	case apiErr.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(apiErr.Body), "invalid_grant"):
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnNeedsReauthorize, Detail: "the authorization was revoked or has expired", Err: err}
	case apiErr.StatusCode == http.StatusForbidden:
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnNeedsReauthorize, Detail: "the provider denied access with the stored credential", Err: err}
	case apiErr.StatusCode == http.StatusNotFound && removedOn404:
		return &gitConnError{Provider: provider, Key: key, Status: store.GitConnRevoked, Detail: "the installation or authorization no longer exists on the provider", Err: err}
	}
	return nil
}

func (rt *Router) recordGitConn(ctx context.Context, e *gitConnError) {
	if rt.gitConn == nil {
		return
	}
	err := rt.gitConn.SetGitConnectionStatus(ctx, store.GitConnectionStatus{
		Provider: e.Key, Status: e.Status, Detail: e.Detail, NextCheckAt: e.NextCheck, UpdatedAt: time.Now(),
	})
	if err != nil {
		rt.logger.Warn("api: record git connection status failed", slog.String("provider", e.Provider), slog.String("error", err.Error()))
	}
}

// markGitConnOK clears a recorded problem after a call that proves the connection works.
func (rt *Router) markGitConnOK(ctx context.Context, key string) {
	if rt.gitConn == nil {
		return
	}
	cur, ok, err := rt.gitConn.GetGitConnectionStatus(ctx, key)
	if err != nil || !ok {
		return
	}
	if err := rt.gitConn.DeleteGitConnectionStatus(ctx, key); err != nil {
		rt.logger.Warn("api: clear git connection status failed", slog.String("connection", key), slog.String("was", cur.Status), slog.String("error", err.Error()))
	}
}

// gitConnRecordFailure classifies err on the credential path, records the
// state and returns the typed error, or err unchanged when it is not a
// connection problem.
func (rt *Router) gitConnRecordFailure(ctx context.Context, provider, key string, err error, removedOn404 bool) error {
	if ce := classifyGitConnErr(provider, key, err, time.Now(), removedOn404); ce != nil {
		rt.recordGitConn(ctx, ce)
		return ce
	}
	return err
}

// gitConnResource is the connection health the API and dashboard show.
type gitConnResource struct {
	Status      string     `json:"status"`
	Detail      string     `json:"detail,omitempty"`
	NextCheckAt *time.Time `json:"next_check_at,omitempty"`
}

func (rt *Router) gitConnHealth(ctx context.Context, key string) gitConnResource {
	res := gitConnResource{Status: store.GitConnOK}
	if rt.gitConn == nil {
		return res
	}
	s, ok, err := rt.gitConn.GetGitConnectionStatus(ctx, key)
	if err != nil || !ok {
		return res
	}
	res.Status, res.Detail = s.Status, s.Detail
	if s.Status == store.GitConnRateLimited {
		if s.NextCheckAt.Before(time.Now()) {
			return gitConnResource{Status: store.GitConnOK}
		}
		next := s.NextCheckAt.UTC()
		res.NextCheckAt = &next
	}
	return res
}

type gitConnErrorBody struct {
	Error       string     `json:"error"`
	Code        string     `json:"code"`
	NextCheckAt *time.Time `json:"next_check_at,omitempty"`
}

// writeGitConnError answers a gitConnError with its 409 (429 for a rate
// limit) and reports whether err was one.
func (rt *Router) writeGitConnError(w http.ResponseWriter, err error) bool {
	var ce *gitConnError
	if !errors.As(err, &ce) {
		return false
	}
	body := gitConnErrorBody{Code: ce.Status}
	status := http.StatusConflict
	switch ce.Status {
	case store.GitConnRateLimited:
		status = http.StatusTooManyRequests
		next := ce.NextCheck.UTC()
		body.NextCheckAt = &next
		w.Header().Set("Retry-After", strconv.Itoa(max(int(time.Until(ce.NextCheck).Seconds())+1, 1)))
		body.Error = ce.Provider + " is rate limiting this control plane"
	case store.GitConnRevoked:
		body.Error = "the " + ce.Provider + " connection was removed on the provider; reconnect it"
	default:
		body.Error = "the " + ce.Provider + " authorization was revoked or has expired; reconnect it"
	}
	writeJSON(w, status, body)
	return true
}

// gitOAuthLocks serializes token refresh per connection, so refreshing one
// account never waits on another.
type gitOAuthLocks struct{ m sync.Map }

func (l *gitOAuthLocks) lock(key string) func() {
	v, _ := l.m.LoadOrStore(key, &sync.Mutex{})
	mu, ok := v.(*sync.Mutex)
	if !ok {
		return func() {}
	}
	mu.Lock()
	return mu.Unlock
}

type gitOAuthState struct {
	Access      string
	Refresh     string
	ExpiresUnix int64
}

type gitOAuthTokens struct {
	Access  string
	Refresh string
	Expires time.Time
}

// gitOAuthSpec is the provider-specific half of a token refresh.
type gitOAuthSpec struct {
	provider string
	key      string
	read     func(ctx context.Context) (gitOAuthState, error)
	exchange func(ctx context.Context, refresh string) (gitOAuthTokens, error)
	save     func(ctx context.Context, t gitOAuthTokens) error
}

const gitOAuthRefreshMargin = 60 * time.Second

// refreshGitOAuth refreshes a connection's access token once at a time. A
// caller that waited on the lock re-reads the stored state and uses the
// token another caller just refreshed; before saving, the refresh token
// that was spent is compared with the stored one so a rotating refresh
// token is never used twice and a reconnect in between is not overwritten.
func (rt *Router) refreshGitOAuth(ctx context.Context, spec gitOAuthSpec) (string, error) {
	unlock := rt.gitOAuthLocks.lock(spec.key)
	defer unlock()

	state, err := spec.read(ctx)
	if err != nil {
		return "", fmt.Errorf("read %s oauth state: %w", spec.provider, err)
	}
	if time.Now().Add(gitOAuthRefreshMargin).Before(time.Unix(state.ExpiresUnix, 0)) {
		return state.Access, nil
	}

	tokens, err := spec.exchange(ctx, state.Refresh)
	if err != nil {
		return "", rt.gitConnRecordFailure(ctx, spec.provider, spec.key, fmt.Errorf("refresh %s access token: %w", spec.provider, err), false)
	}
	if tokens.Refresh == "" {
		tokens.Refresh = state.Refresh
	}

	current, err := spec.read(ctx)
	if err != nil {
		return "", fmt.Errorf("re-read %s oauth state: %w", spec.provider, err)
	}
	if current.Refresh != state.Refresh {
		if time.Now().Add(gitOAuthRefreshMargin).Before(time.Unix(current.ExpiresUnix, 0)) {
			return current.Access, nil
		}
		ce := &gitConnError{Provider: spec.provider, Key: spec.key, Status: store.GitConnNeedsReauthorize, Detail: "the connection changed while refreshing", Err: errors.New("refresh token changed during refresh")}
		rt.recordGitConn(ctx, ce)
		return "", ce
	}
	if err := spec.save(ctx, tokens); err != nil {
		return "", fmt.Errorf("save refreshed %s tokens: %w", spec.provider, err)
	}
	rt.markGitConnOK(ctx, spec.key)
	return tokens.Access, nil
}

func (rt *Router) attachGitConnStore(s Store) {
	if gs, ok := any(s).(gitConnectionStore); ok {
		rt.gitConn = gs
	}
}

// gitOAuthSecrets is the slice of the secrets manager the refresh state read needs.
type gitOAuthSecrets interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

func (rt *Router) readGitOAuthState(ctx context.Context, sec gitOAuthSecrets, key, accessKey, refreshKey, expiresKey string) (gitOAuthState, error) {
	access, err := sec.Resolve(ctx, key, accessKey)
	if err != nil {
		return gitOAuthState{}, fmt.Errorf("resolve access token: %w", err)
	}
	refresh, err := sec.Resolve(ctx, key, refreshKey)
	if err != nil {
		return gitOAuthState{}, fmt.Errorf("resolve refresh token: %w", err)
	}
	raw, err := sec.Resolve(ctx, key, expiresKey)
	if err != nil {
		return gitOAuthState{}, fmt.Errorf("resolve token expiry: %w", err)
	}
	exp, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return gitOAuthState{}, fmt.Errorf("parse token expiry: %w", err)
	}
	return gitOAuthState{Access: access, Refresh: refresh, ExpiresUnix: exp}, nil
}

// gitDisconnectResource tells the operator what a disconnect did on the
// provider's side, not just locally.
type gitDisconnectResource struct {
	Disconnected bool `json:"disconnected"`
	// TokenRevoked is true only when the provider's own revocation call succeeded.
	TokenRevoked bool   `json:"token_revoked"`
	Note         string `json:"note"`
	// AffectedSources names git sources that used the connection; they stay and need a reconnect.
	AffectedSources []string `json:"affected_sources,omitempty"`
}

func (rt *Router) finishGitDisconnect(ctx context.Context, w http.ResponseWriter, key string, revoked bool, note string, sources []string) {
	if rt.gitConn != nil {
		if err := rt.gitConn.DeleteGitConnectionStatus(ctx, key); err != nil {
			rt.logger.Warn("api: clear git connection status on disconnect failed", slog.String("connection", key), slog.String("error", err.Error()))
		}
	}
	writeJSON(w, http.StatusOK, gitDisconnectResource{Disconnected: true, TokenRevoked: revoked, Note: note, AffectedSources: sources})
}

const gitRevokeTimeout = 10 * time.Second
