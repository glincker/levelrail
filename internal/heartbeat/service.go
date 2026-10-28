package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	secretsService = "heartbeat"
	secretURL      = "url"
	secretFailURL  = "fail_url"
	maxRedirects   = 3
)

// Result words recorded after each tick.
const (
	ResultOK         = "ok"
	ResultFailSent   = "fail_reported"
	ResultUnhealthy  = "unhealthy"
	ResultSendFailed = "error"
)

// Source says where the active ping URL comes from.
const (
	SourceEnv       = "env"
	SourceDashboard = "dashboard"
)

var (
	// ErrNoSecrets means no master key is configured, so a URL cannot be stored.
	ErrNoSecrets = errors.New("heartbeat: no secrets store configured")
	// ErrEnvManaged means APP_HEARTBEAT_URL is set and owns the configuration.
	ErrEnvManaged = errors.New("heartbeat: configured by " + EnvURL)
)

// Secrets is the slice of the secrets manager the heartbeat needs.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// StateStore persists the last ping outcome and the body toggle.
type StateStore interface {
	GetHeartbeatState(ctx context.Context) (store.HeartbeatState, error)
	SaveHeartbeatState(ctx context.Context, s store.HeartbeatState) error
}

// Health reports whether the control plane is fit to claim it is alive, with a short reason word when not.
type Health func(ctx context.Context) (ok bool, reason string)

// Options carries the service's collaborators.
type Options struct {
	Config    Config
	Secrets   Secrets
	State     StateStore
	Health    Health
	Logger    *slog.Logger
	Version   string
	UserAgent string
}

// Service pings the configured URL while the control plane is healthy.
type Service struct {
	cfg     Config
	secrets Secrets
	state   StateStore
	health  Health
	logger  *slog.Logger
	version string
	agent   string
	started time.Time
	client  *http.Client
	wake    chan struct{}
	mu      sync.Mutex
}

// New builds a Service. Secrets may be nil, which limits configuration to the env.
func New(o Options) *Service {
	logger := o.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Service{cfg: o.Config, secrets: o.Secrets, state: o.State, health: o.Health, logger: logger,
		version: o.Version, agent: o.UserAgent, started: time.Now(), wake: make(chan struct{}, 1)}
	s.client = &http.Client{
		Timeout: o.Config.Timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         guardedDialer(o.Config.AllowPrivate, o.Config.Timeout).DialContext,
			TLSHandshakeTimeout: o.Config.Timeout,
			DisableKeepAlives:   true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errors.New("too many redirects")
			}
			if !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) || (via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
				return errors.New("redirect to another host refused")
			}
			return nil
		},
	}
	return s
}

type target struct{ url, failURL, source string }

func (s *Service) target(ctx context.Context) (target, error) {
	if s.cfg.URL != "" {
		return target{url: s.cfg.URL, failURL: s.cfg.FailURL, source: SourceEnv}, nil
	}
	if s.secrets == nil {
		return target{}, nil
	}
	ok, err := s.secrets.Exists(ctx, secretsService, secretURL)
	if err != nil || !ok {
		return target{}, err
	}
	u, err := s.secrets.Resolve(ctx, secretsService, secretURL)
	if err != nil {
		return target{}, err
	}
	t := target{url: u, source: SourceDashboard}
	if fok, ferr := s.secrets.Exists(ctx, secretsService, secretFailURL); ferr == nil && fok {
		t.failURL, _ = s.secrets.Resolve(ctx, secretsService, secretFailURL)
	}
	return t, nil
}

// Run pings every interval until ctx ends. It re-reads the target each tick so dashboard edits apply without a restart.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
	}
}

// Tick runs one health check and, when a target exists, one ping.
func (s *Service) Tick(ctx context.Context) {
	tg, err := s.target(ctx)
	if err != nil {
		s.logger.Warn("heartbeat: target lookup failed", slog.String("error", err.Error()))
		return
	}
	if tg.url == "" {
		return
	}
	ok, reason := true, ""
	if s.health != nil {
		ok, reason = s.health(ctx)
	}
	result, detail := ResultOK, ""
	var sendErr error
	switch {
	case ok:
		sendErr = s.send(ctx, tg.url, true, "")
	case tg.failURL != "":
		result = ResultFailSent
		detail = reason
		sendErr = s.send(ctx, tg.failURL, false, reason)
	default:
		result, detail = ResultUnhealthy, reason
	}
	if sendErr != nil {
		result, detail = ResultSendFailed, sendErr.Error()
		s.logger.Warn("heartbeat: ping failed", slog.String("host", HostOf(tg.url)), slog.String("error", detail))
	}
	s.record(ctx, result, detail)
}

func (s *Service) bodyOn(ctx context.Context) bool {
	if s.cfg.BodyOff {
		return false
	}
	if s.state == nil {
		return true
	}
	st, err := s.state.GetHeartbeatState(ctx)
	return err != nil || st.BodyEnabled
}

func (s *Service) send(ctx context.Context, raw string, healthy bool, reason string) error {
	if _, err := ValidateURL(raw, s.cfg.AllowPrivate); err != nil {
		return err
	}
	var body io.Reader
	method := http.MethodGet
	if s.cfg.Method == http.MethodPost {
		method = http.MethodPost
		if s.bodyOn(ctx) {
			b, _ := json.Marshal(s.payload(healthy, reason))
			body = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return scrub(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.agent != "" {
		req.Header.Set("User-Agent", s.agent)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return scrub(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("monitor answered HTTP %d", resp.StatusCode)
	}
	return nil
}

// payload is status words, the version and the uptime: no hostnames, no ids.
func (s *Service) payload(healthy bool, reason string) map[string]any {
	p := map[string]any{"status": "ok", "version": s.version, "uptime_seconds": int64(time.Since(s.started).Seconds())}
	if !healthy {
		p["status"] = "degraded"
		p["reason"] = reason
	}
	return p
}

// scrub drops the URL net/http wraps into its errors because it contains the monitor's secret key.
func scrub(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return errors.New(ue.Err.Error())
	}
	return err
}

func (s *Service) record(ctx context.Context, result, detail string) {
	if s.state == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.state.GetHeartbeatState(ctx)
	if err != nil {
		return
	}
	st.LastPingAt, st.LastResult, st.LastError = time.Now().UTC(), result, detail
	if err := s.state.SaveHeartbeatState(ctx, st); err != nil {
		s.logger.Warn("heartbeat: saving state failed", slog.String("error", err.Error()))
	}
}

// Status is the API view of the heartbeat; it never carries a URL, only hosts.
type Status struct {
	Configured   bool      `json:"configured"`
	Source       string    `json:"source,omitempty"`
	Host         string    `json:"host,omitempty"`
	FailHost     string    `json:"fail_host,omitempty"`
	IntervalSecs int       `json:"interval_seconds"`
	BodyEnabled  bool      `json:"body_enabled"`
	CanStoreURL  bool      `json:"can_store_url"`
	LastPingAt   time.Time `json:"last_ping_at,omitzero"`
	LastResult   string    `json:"last_result,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

// Status reports the current configuration and last outcome.
func (s *Service) Status(ctx context.Context) (Status, error) {
	tg, err := s.target(ctx)
	if err != nil {
		return Status{}, err
	}
	out := Status{Configured: tg.url != "", Source: tg.source, Host: HostOf(tg.url), FailHost: HostOf(tg.failURL),
		IntervalSecs: int(s.cfg.Interval.Seconds()), BodyEnabled: !s.cfg.BodyOff, CanStoreURL: s.secrets != nil && s.cfg.URL == ""}
	if s.state != nil {
		st, serr := s.state.GetHeartbeatState(ctx)
		if serr != nil {
			return Status{}, serr
		}
		out.BodyEnabled = out.BodyEnabled && st.BodyEnabled
		out.LastPingAt, out.LastResult, out.LastError = st.LastPingAt, st.LastResult, st.LastError
	}
	return out, nil
}

// Update is a partial change: nil fields stay as they are, an empty URL string clears that URL.
type Update struct {
	URL     *string
	FailURL *string
	Body    *bool
}

// Apply validates and stores an Update, then pings at once so the monitor sees the new URL.
func (s *Service) Apply(ctx context.Context, u Update) error {
	if s.cfg.URL != "" {
		return ErrEnvManaged
	}
	if s.secrets == nil {
		return ErrNoSecrets
	}
	for _, f := range []struct {
		v   *string
		key string
	}{{u.URL, secretURL}, {u.FailURL, secretFailURL}} {
		if f.v == nil {
			continue
		}
		v := strings.TrimSpace(*f.v)
		if v == "" {
			if err := s.secrets.DeleteValue(ctx, secretsService, f.key); err != nil {
				return err
			}
			continue
		}
		if _, err := ValidateURL(v, s.cfg.AllowPrivate); err != nil {
			return err
		}
		if err := s.secrets.SetValue(ctx, secretsService, f.key, v); err != nil {
			return err
		}
	}
	if u.URL != nil && strings.TrimSpace(*u.URL) == "" {
		_ = s.secrets.DeleteValue(ctx, secretsService, secretFailURL)
	}
	if u.Body != nil && s.state != nil {
		s.mu.Lock()
		st, err := s.state.GetHeartbeatState(ctx)
		if err == nil {
			st.BodyEnabled = *u.Body
			err = s.state.SaveHeartbeatState(ctx, st)
		}
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}
