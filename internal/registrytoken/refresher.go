package registrytoken

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env names for the refresher; durations use Go syntax such as 30m.
const (
	EnvLead        = "APP_REGISTRY_REFRESH_LEAD"
	EnvInterval    = "APP_REGISTRY_REFRESH_INTERVAL"
	EnvRetry       = "APP_REGISTRY_REFRESH_RETRY"
	EnvFallbackTTL = "APP_REGISTRY_ACR_FALLBACK_TTL"

	defaultLead        = 30 * time.Minute
	defaultInterval    = time.Minute
	defaultRetry       = 5 * time.Minute
	defaultFallbackTTL = 3 * time.Hour
)

// Settings are the refresher timings.
type Settings struct {
	Lead, Interval, Retry, FallbackTTL time.Duration
}

// SettingsFromEnv reads timings from lookup; invalid values keep defaults.
func SettingsFromEnv(lookup func(string) string) Settings {
	get := func(key string, def time.Duration) time.Duration {
		d, err := time.ParseDuration(strings.TrimSpace(lookup(key)))
		if err != nil || d <= 0 {
			return def
		}
		return d
	}
	return Settings{
		Lead:        get(EnvLead, defaultLead),
		Interval:    get(EnvInterval, defaultInterval),
		Retry:       get(EnvRetry, defaultRetry),
		FallbackTTL: get(EnvFallbackTTL, defaultFallbackTTL),
	}
}

// CredentialStore is the registry credential store surface the refresher uses.
type CredentialStore interface {
	ListRegistryCredentials(ctx context.Context) ([]store.RegistryCredential, error)
	GetRegistryCredential(ctx context.Context, id string) (store.RegistryCredential, error)
	RecordRegistryCredentialRefresh(ctx context.Context, id, registryHost, username string, expiresAt, refreshedAt, nextAt *time.Time, errMsg string) error
}

// SecretStore is the envelope-encrypted secret surface; *secrets.Manager satisfies it.
type SecretStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Refresher renews preset registry credentials before they expire.
type Refresher struct {
	Store    CredentialStore
	Secrets  SecretStore
	Minters  map[string]Minter
	Settings Settings
	Now      func() time.Time
	Logger   *slog.Logger

	// mu serialises refreshes so the API button and the loop never race a write.
	mu sync.Mutex
}

func (r *Refresher) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// ParseConfig decodes a credential's stored preset_config.
func ParseConfig(raw string) (Config, error) {
	var cfg Config
	if raw == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, fmt.Errorf("registrytoken: decode preset config: %w", err)
	}
	return cfg, nil
}

// EncodeConfig serialises a Config for preset_config.
func EncodeConfig(cfg Config) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("registrytoken: encode preset config: %w", err)
	}
	return string(b), nil
}

// Preview mints a login without storing anything, for the Test button.
func (r *Refresher) Preview(ctx context.Context, preset string, cfg Config) (Minted, error) {
	provider, err := ProviderFor(preset)
	if err != nil {
		return Minted{}, err
	}
	minter, ok := r.Minters[preset]
	if !ok {
		return Minted{}, fmt.Errorf("registrytoken: no minter for preset %q", preset)
	}
	cloud, err := r.Secrets.Resolve(ctx, store.NodeProviderSecretsKey(provider), store.NodeProviderTokenEnvKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return Minted{}, ErrNoCloudConnection
	}
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: resolve %s connection: %w", provider, err)
	}
	return minter.Mint(ctx, cloud, cfg)
}

// Interval is how often the loop checks for due credentials.
func (r *Refresher) Interval() time.Duration { return r.Settings.Interval }

// NextRefresh is when a login expiring at expires should be renewed.
func (r *Refresher) NextRefresh(now, expires time.Time) time.Time {
	next := expires.Add(-r.Settings.Lead)
	if !next.After(now) {
		// Short-lived token: renew at half of what remains instead of spinning.
		next = now.Add(expires.Sub(now) / 2)
	}
	return next
}

// Refresh mints a new login for one credential and stores it. A failure is
// recorded on the row (so the alert and UI show it) and returned.
func (r *Refresher) Refresh(ctx context.Context, id string) (store.RegistryCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cred, err := r.Store.GetRegistryCredential(ctx, id)
	if err != nil {
		return store.RegistryCredential{}, fmt.Errorf("registrytoken: load credential %q: %w", id, err)
	}
	if cred.Preset == "" {
		return cred, fmt.Errorf("registrytoken: credential %q is not a preset credential", id)
	}
	now := r.now()
	minted, err := r.mint(ctx, cred)
	if err != nil {
		next := now.Add(r.Settings.Retry)
		if rerr := r.Store.RecordRegistryCredentialRefresh(ctx, id, "", "", nil, nil, &next, SafeError(err)); rerr != nil {
			r.Logger.Error("registry credential refresh: record failure failed", slog.String("id", id), slog.String("error", rerr.Error()))
		}
		r.Logger.Warn("registry credential refresh failed", slog.String("id", id), slog.String("preset", cred.Preset), slog.String("error", SafeError(err)))
		return cred, err
	}

	// SetValue is one encrypted upsert, so a concurrent pull reads the old or
	// the new password, never a partial one; the metadata follows in one UPDATE.
	if err := r.Secrets.SetValue(ctx, store.RegistryCredentialSecretsKey(id), "password", minted.Password); err != nil {
		return cred, fmt.Errorf("registrytoken: store refreshed password for %q: %w", id, err)
	}
	next := r.NextRefresh(now, minted.ExpiresAt)
	exp := minted.ExpiresAt
	if err := r.Store.RecordRegistryCredentialRefresh(ctx, id, minted.RegistryHost, minted.Username, &exp, &now, &next, ""); err != nil {
		return cred, fmt.Errorf("registrytoken: record refresh for %q: %w", id, err)
	}
	r.Logger.Info("registry credential refreshed", slog.String("id", id), slog.String("preset", cred.Preset), slog.Time("expires_at", exp), slog.Time("next_refresh_at", next))
	return r.Store.GetRegistryCredential(ctx, id)
}

func (r *Refresher) mint(ctx context.Context, cred store.RegistryCredential) (Minted, error) {
	cfg, err := ParseConfig(cred.PresetConfig)
	if err != nil {
		return Minted{}, err
	}
	if cfg.RegistryHost == "" {
		cfg.RegistryHost = cred.RegistryHost
	}
	return r.Preview(ctx, cred.Preset, cfg)
}

// Tick refreshes every preset credential whose next refresh time has passed.
func (r *Refresher) Tick(ctx context.Context) error {
	creds, err := r.Store.ListRegistryCredentials(ctx)
	if err != nil {
		return fmt.Errorf("registrytoken: list credentials: %w", err)
	}
	now := r.now()
	for _, c := range creds {
		if c.Preset == "" || (c.NextRefreshAt != nil && now.Before(*c.NextRefreshAt)) {
			continue
		}
		if _, err := r.Refresh(ctx, c.ID); err != nil && ctx.Err() != nil {
			return fmt.Errorf("registrytoken: refresh interrupted: %w", ctx.Err())
		}
	}
	return nil
}
