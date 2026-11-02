package ddns

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Secrets is the encrypted store the setting lives in (the secrets manager satisfies it).
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

const (
	secretsService = "ddns"
	keySettings    = "settings"
	keyTarget      = "target"

	// ProviderGeneric is an operator-supplied update URL with an {ip} placeholder.
	ProviderGeneric = "generic"
	// ProviderDuckDNS is the DuckDNS preset (domains plus token).
	ProviderDuckDNS = "duckdns"

	// SourceStored says the setting came from the stored row.
	SourceStored = "stored"
	// SourceDefault says nothing is stored, so env defaults apply.
	SourceDefault = "default"
)

// ErrNoSecrets means no master key is configured, so nothing can be stored.
var ErrNoSecrets = errors.New("storing the dynamic DNS target needs a configured master key")

// Settings is the stored, non-secret part. Durations are whole seconds.
type Settings struct {
	Enabled           bool     `json:"enabled"`
	Provider          string   `json:"provider"`
	Domains           string   `json:"domains,omitempty"`
	IntervalSecs      int      `json:"interval_seconds"`
	ProbeURLs         []string `json:"probe_urls"`
	ProbeQuorum       int      `json:"probe_quorum"`
	TimeoutSecs       int      `json:"timeout_seconds"`
	MinUpdateGapSecs  int      `json:"min_update_gap_seconds"`
	ReassertSecs      int      `json:"reassert_seconds"`
	FailAlertAfter    int      `json:"fail_alert_after"`
	AllowPrivate      bool     `json:"allow_private"`
	AllowInsecureHTTP bool     `json:"allow_insecure_http"`
}

// Update is a partial change; nil fields keep the current value, a non-nil empty Target clears it.
type Update struct {
	Enabled           *bool
	Provider          *string
	Domains           *string
	Target            *string
	IntervalSecs      *int
	ProbeURLs         *[]string
	ProbeQuorum       *int
	TimeoutSecs       *int
	MinUpdateGapSecs  *int
	ReassertSecs      *int
	FailAlertAfter    *int
	AllowPrivate      *bool
	AllowInsecureHTTP *bool
}

// Resource is what the API returns: settings, where they came from, and the host only, never the target.
type Resource struct {
	Settings
	Source           string `json:"source"`
	TargetConfigured bool   `json:"target_configured"`
	Host             string `json:"host,omitempty"`
	Running          bool   `json:"running"`
}

// StatusView is the loop state for the status read.
type StatusView struct {
	Running     bool      `json:"running"`
	LastCheck   time.Time `json:"last_check,omitzero"`
	LastChange  time.Time `json:"last_change,omitzero"`
	LastIP      string    `json:"last_ip,omitempty"`
	LastResult  string    `json:"last_result"`
	LastError   string    `json:"last_error,omitempty"`
	Failures    int       `json:"consecutive_failures"`
	AlertActive bool      `json:"alert_active"`
}

// DefaultSettings seeds the setting from env; it applies only until something is stored.
func DefaultSettings(getenv func(string) string) Settings {
	c := ConfigFromEnv(getenv)
	s := Settings{
		Enabled: Enabled(getenv), Provider: ProviderGeneric, IntervalSecs: int(c.Interval / time.Second),
		ProbeURLs: c.ProbeURLs, ProbeQuorum: c.Quorum, TimeoutSecs: int(c.Timeout / time.Second),
		MinUpdateGapSecs: int(c.MinUpdateGap / time.Second), ReassertSecs: int(c.Reassert / time.Second),
		FailAlertAfter: c.FailAlertAfter, AllowPrivate: c.AllowPrivate, AllowInsecureHTTP: c.AllowInsecure,
	}
	if strings.TrimSpace(getenv(EnvUpdateURL)) == "" && strings.TrimSpace(getenv(EnvDuckDomains)) != "" {
		s.Provider, s.Domains = ProviderDuckDNS, strings.TrimSpace(getenv(EnvDuckDomains))
	}
	return s
}

// Config converts the stored seconds into loop settings.
func (s Settings) Config() Config {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return Config{Interval: sec(s.IntervalSecs), ProbeURLs: s.ProbeURLs, Quorum: s.ProbeQuorum, Timeout: sec(s.TimeoutSecs),
		MinUpdateGap: sec(s.MinUpdateGapSecs), Reassert: sec(s.ReassertSecs), FailAlertAfter: s.FailAlertAfter,
		AllowPrivate: s.AllowPrivate, AllowInsecure: s.AllowInsecureHTTP}
}

func (s Settings) validate() error {
	if s.Provider != ProviderGeneric && s.Provider != ProviderDuckDNS {
		return errors.New("provider must be generic or duckdns")
	}
	for name, n := range map[string]int{"interval_seconds": s.IntervalSecs, "probe_quorum": s.ProbeQuorum, "timeout_seconds": s.TimeoutSecs,
		"min_update_gap_seconds": s.MinUpdateGapSecs, "reassert_seconds": s.ReassertSecs, "fail_alert_after": s.FailAlertAfter} {
		if n < 1 {
			return fmt.Errorf("%s must be at least 1", name)
		}
	}
	if len(s.ProbeURLs) == 0 || s.ProbeQuorum > len(s.ProbeURLs) {
		return errors.New("probe_quorum cannot exceed the number of probe_urls")
	}
	for _, p := range s.ProbeURLs {
		u, err := url.Parse(p)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && (u.Scheme != "http" || !s.AllowInsecureHTTP)) {
			return errors.New("probe_urls must be https URLs")
		}
		if !s.AllowPrivate {
			if err := netguard.ValidateURL(context.Background(), p); err != nil {
				return errors.New("a probe url destination is not allowed")
			}
		}
	}
	return nil
}

// Manager owns the stored setting and keeps one updater loop matching it.
type Manager struct {
	secrets Secrets
	getenv  func(string) string
	log     *slog.Logger
	hooks   Hooks
	parent  context.Context

	mu     sync.Mutex
	cancel context.CancelFunc
	up     *Updater
	sig    [32]byte
	// carry survives a loop restart in memory only; tsig ties it to the provider and target.
	carry Carry
	tsig  [32]byte
}

// NewManager wires the manager; secrets may be nil, then only env defaults apply and nothing can be stored.
func NewManager(ctx context.Context, s Secrets, getenv func(string) string, log *slog.Logger, hooks Hooks) *Manager {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Manager{secrets: s, getenv: getenv, log: log, hooks: hooks, parent: ctx}
}

func (m *Manager) load(ctx context.Context) (Settings, string, error) {
	def := DefaultSettings(m.getenv)
	if m.secrets == nil {
		return def, SourceDefault, nil
	}
	ok, err := m.secrets.Exists(ctx, secretsService, keySettings)
	if err != nil || !ok {
		return def, SourceDefault, err
	}
	raw, err := m.secrets.Resolve(ctx, secretsService, keySettings)
	if err != nil {
		return def, SourceDefault, err
	}
	s := def
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return def, SourceDefault, errors.New("stored dynamic DNS setting is unreadable")
	}
	return s, SourceStored, nil
}

// target returns the stored URL or token, falling back to the env secret only for generic and DuckDNS defaults.
func (m *Manager) target(ctx context.Context) (string, bool, error) {
	if m.secrets == nil {
		return "", false, nil
	}
	ok, err := m.secrets.Exists(ctx, secretsService, keyTarget)
	if err != nil || !ok {
		return "", false, err
	}
	v, err := m.secrets.Resolve(ctx, secretsService, keyTarget)
	return v, v != "", err
}

func buildProvider(s Settings, target string) (Provider, error) {
	cfg := s.Config()
	switch s.Provider {
	case ProviderDuckDNS:
		if strings.TrimSpace(s.Domains) == "" || target == "" {
			return nil, errors.New("duckdns needs domains and a token")
		}
		return NewDuckDNS(s.Domains, target, cfg)
	default:
		if target == "" {
			return nil, ErrNoTarget
		}
		return NewHTTPProvider(target, cfg, nil)
	}
}

// provider builds the effective provider: the stored target wins, env fills in only when nothing is stored.
func (m *Manager) provider(ctx context.Context, s Settings, src string) (Provider, error) {
	t, ok, err := m.target(ctx)
	if err != nil {
		return nil, err
	}
	if !ok && src == SourceDefault {
		return ProviderFromEnv(m.getenv, s.Config())
	}
	return buildProvider(s, t)
}

// Get returns the effective setting. The target is never part of it.
func (m *Manager) Get(ctx context.Context) (Resource, error) {
	s, src, err := m.load(ctx)
	if err != nil {
		return Resource{}, err
	}
	t, ok, err := m.target(ctx)
	if err != nil {
		return Resource{}, err
	}
	res := Resource{Settings: s, Source: src, TargetConfigured: ok}
	if !ok && src == SourceDefault {
		if u := strings.TrimSpace(m.getenv(EnvUpdateURL)); u != "" {
			t, res.TargetConfigured = u, true
		} else if m.getenv(EnvDuckToken) != "" && s.Domains != "" {
			res.TargetConfigured = true
		}
	}
	res.Host = hostOf(s, t)
	m.mu.Lock()
	res.Running = m.up != nil
	m.mu.Unlock()
	return res, nil
}

func hostOf(s Settings, target string) string {
	if s.Provider == ProviderDuckDNS {
		return "www.duckdns.org"
	}
	if u, err := url.Parse(strings.ReplaceAll(target, ipPlaceholder, "203.0.113.1")); err == nil {
		return u.Hostname()
	}
	return ""
}

// Apply validates and stores a change, then reconciles the loop so it takes effect without a restart.
func (m *Manager) Apply(ctx context.Context, u Update) (Resource, error) {
	if m.secrets == nil {
		return Resource{}, ErrNoSecrets
	}
	s, _, err := m.load(ctx)
	if err != nil {
		return Resource{}, err
	}
	setIf(&s.Enabled, u.Enabled)
	setIf(&s.Provider, u.Provider)
	setIf(&s.Domains, u.Domains)
	setIf(&s.IntervalSecs, u.IntervalSecs)
	setIf(&s.ProbeURLs, u.ProbeURLs)
	setIf(&s.ProbeQuorum, u.ProbeQuorum)
	setIf(&s.TimeoutSecs, u.TimeoutSecs)
	setIf(&s.MinUpdateGapSecs, u.MinUpdateGapSecs)
	setIf(&s.ReassertSecs, u.ReassertSecs)
	setIf(&s.FailAlertAfter, u.FailAlertAfter)
	setIf(&s.AllowPrivate, u.AllowPrivate)
	setIf(&s.AllowInsecureHTTP, u.AllowInsecureHTTP)
	s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	if err := s.validate(); err != nil {
		return Resource{}, err
	}
	target, has, err := m.target(ctx)
	if err != nil {
		return Resource{}, err
	}
	if u.Target != nil {
		target, has = strings.TrimSpace(*u.Target), strings.TrimSpace(*u.Target) != ""
	}
	if has || s.Enabled {
		if _, err := buildProvider(s, target); err != nil {
			return Resource{}, err
		}
	}
	raw, _ := json.Marshal(s)
	if err := m.secrets.SetValue(ctx, secretsService, keySettings, string(raw)); err != nil {
		return Resource{}, errors.New("could not store the setting")
	}
	if u.Target != nil {
		if has {
			err = m.secrets.SetValue(ctx, secretsService, keyTarget, target)
		} else {
			err = m.secrets.DeleteValue(ctx, secretsService, keyTarget)
		}
		if err != nil {
			return Resource{}, errors.New("could not store the target")
		}
	}
	if err := m.Reconcile(ctx); err != nil {
		m.log.Warn("ddns reconcile after change failed", "error", err.Error())
	}
	return m.Get(ctx)
}

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

// Reconcile starts, restarts or stops the loop so it matches the effective setting.
func (m *Manager) Reconcile(ctx context.Context) error {
	s, src, err := m.load(ctx)
	if err != nil {
		return err
	}
	var p Provider
	if s.Enabled {
		if p, err = m.provider(ctx, s, src); err != nil {
			m.stop()
			return err
		}
	}
	raw, _ := json.Marshal(s)
	t, _, _ := m.target(ctx)
	sig := sha256.Sum256(append(raw, t...))
	m.mu.Lock()
	defer m.mu.Unlock()
	if !s.Enabled {
		m.stopLocked()
		return nil
	}
	if m.up != nil && m.sig == sig {
		return nil
	}
	m.stopLocked()
	cfg := s.Config()
	up := New(cfg, NewDetector(cfg), p, m.log, m.hooks)
	// A changed provider or target must write at once; any other change keeps the gap.
	tsig := sha256.Sum256([]byte(s.Provider + "\x00" + s.Domains + "\x00" + t))
	if tsig == m.tsig {
		up.Seed(m.carry)
	} else {
		m.carry = Carry{}
	}
	m.tsig = tsig
	runCtx, cancel := context.WithCancel(m.parent)
	m.up, m.cancel, m.sig = up, cancel, sig
	go up.Run(runCtx)
	return nil
}

func (m *Manager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.up != nil {
		if c := m.up.Carry(); c.LastIP != "" {
			m.carry = c
		}
	}
	m.up, m.cancel = nil, nil
}

// Status returns the loop snapshot; Running is false when the loop is stopped.
func (m *Manager) Status() StatusView {
	m.mu.Lock()
	up := m.up
	m.mu.Unlock()
	if up == nil {
		return StatusView{LastResult: "not running"}
	}
	st := up.Status()
	v := StatusView{Running: true, LastCheck: st.LastCheck, LastChange: st.LastChange, LastIP: st.LastIP, LastError: st.LastError,
		Failures: st.Failures, AlertActive: st.AlertActive, LastResult: "ok"}
	switch {
	case st.LastCheck.IsZero():
		v.LastResult = "pending"
	case st.LastError != "":
		v.LastResult = "failing"
	}
	return v
}
