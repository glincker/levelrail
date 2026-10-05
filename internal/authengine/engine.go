// Package authengine hosts the theauth-go library behind a flag. It is the
// only package that imports the library, so a module path change is a one-line sweep.
package authengine

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
)

const (
	// EnvEngine selects the auth engine; only EngineLibrary mounts the library.
	EnvEngine = "APP_AUTH_ENGINE"
	// EngineLegacy is the default EnvEngine value: nothing new is mounted.
	EngineLegacy = "legacy"
	// EngineLibrary is the EnvEngine value that mounts the library handler.
	EngineLibrary = "library"

	// EnvPathPrefix overrides DefaultPathPrefix.
	EnvPathPrefix = "APP_AUTH_ENGINE_PATH_PREFIX"
	// EnvBaseURL overrides the externally visible base URL.
	EnvBaseURL = "APP_AUTH_ENGINE_BASE_URL"
	// EnvPasswordMinLength overrides the minimum password length.
	EnvPasswordMinLength = "APP_AUTH_ENGINE_PASSWORD_MIN_LENGTH" //nolint:gosec // env var name, not a credential
	// EnvPollInterval overrides the device grant poll interval (Go duration).
	EnvPollInterval = "APP_AUTH_ENGINE_DEVICE_POLL_INTERVAL"
	// EnvRateLimitIP overrides the per-IP request budget.
	EnvRateLimitIP = "APP_AUTH_ENGINE_RATE_LIMIT_PER_IP"

	// DefaultPathPrefix is where the library routes mount until the cutover.
	DefaultPathPrefix = "/api/v1/auth-lib"

	defaultMinPasswordLength = 8
	defaultPollInterval      = 5 * time.Second
	cookieName               = "authx_session"
)

// Mode returns the engine selected by APP_AUTH_ENGINE; anything but "library" is legacy.
func Mode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(EnvEngine)), EngineLibrary) {
		return EngineLibrary
	}
	return EngineLegacy
}

// Enabled reports whether the library engine is selected.
func Enabled() bool { return Mode() == EngineLibrary }

// Config is everything New needs beyond the database.
type Config struct {
	BaseURL         string
	PathPrefix      string
	SetupToken      string
	TokenPrefix     string
	TOTPIssuer      string
	EncryptionKey   []byte
	PollInterval    time.Duration
	RateLimitPerIP  int
	MinPasswordLen  int
	Directory       *Directory
	DeviceVerifyURL string
	MFA             MFAConfig
}

// ConfigFromEnv applies the env overrides on top of base and fills defaults.
func ConfigFromEnv(base Config) Config {
	if v := os.Getenv(EnvBaseURL); v != "" {
		base.BaseURL = v
	}
	if v := os.Getenv(EnvPathPrefix); v != "" {
		base.PathPrefix = v
	}
	if v := os.Getenv(EnvPollInterval); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			base.PollInterval = d
		}
	}
	if v := os.Getenv(EnvRateLimitIP); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			base.RateLimitPerIP = n
		}
	}
	if v := os.Getenv(EnvPasswordMinLength); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			base.MinPasswordLen = n
		}
	}
	if base.PathPrefix == "" {
		base.PathPrefix = DefaultPathPrefix
	}
	if base.PollInterval == 0 {
		base.PollInterval = defaultPollInterval
	}
	if base.MinPasswordLen == 0 {
		base.MinPasswordLen = defaultMinPasswordLength
	}
	return base
}

// Engine wraps one theauth instance.
type Engine struct {
	auth     *theauth.TheAuth
	prefix   string
	totp     bool
	passkeys bool
}

// New builds the engine over db. The library tables must exist already: they
// come from the store migration that embeds the library schema.
func New(db *sql.DB, cfg Config) (*Engine, error) {
	if db == nil {
		return nil, errors.New("authengine: nil database")
	}
	if cfg.Directory == nil {
		return nil, errors.New("authengine: directory is required")
	}
	cfg = ConfigFromEnv(cfg)
	store, err := sqlitestore.New(db)
	if err != nil {
		return nil, fmt.Errorf("authengine: storage: %w", err)
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	verify := cfg.DeviceVerifyURL
	if verify == "" {
		verify = base + "/device"
	}
	tcfg := theauth.Config{
		CoreStorage:                   store,
		BaseURL:                       base,
		PathPrefix:                    cfg.PathPrefix,
		CookieName:                    cookieName,
		RateLimitPerIP:                cfg.RateLimitPerIP,
		SecureCookie:                  strings.HasPrefix(base, "https://"),
		SuppressSecureCookieWarning:   true,
		SuppressTrustedProxiesWarning: true,
		PasswordPolicy: theauth.PasswordPolicyConfig{
			MinLength:         cfg.MinPasswordLen,
			AllowLegacyBcrypt: true,
		},
		APITokens: &theauth.APITokensConfig{
			Prefix:           cfg.TokenPrefix,
			AcceptUnprefixed: true,
			Abilities:        EngineAbilities(),
			UserAbilities:    cfg.Directory.UserAbilities,
			IsAdmin:          cfg.Directory.IsAdmin,
			OwnerActive:      cfg.Directory.OwnerActive,
			Device: &theauth.DeviceConfig{
				VerificationURI:  verify,
				DefaultAbilities: []string{AbilityRead},
				Interval:         cfg.PollInterval,
			},
		},
	}
	if cfg.SetupToken != "" {
		tcfg.Bootstrap = &theauth.BootstrapConfig{SetupToken: cfg.SetupToken}
	}
	if len(cfg.EncryptionKey) > 0 {
		tcfg.EncryptionKey = cfg.EncryptionKey
		tcfg.TOTP = &theauth.TOTPConfig{Issuer: cfg.TOTPIssuer}
	}
	applyMFA(&tcfg, cfg)
	a, err := theauth.New(tcfg)
	if err != nil {
		return nil, fmt.Errorf("authengine: init: %w", err)
	}
	return &Engine{auth: a, prefix: cfg.PathPrefix, totp: tcfg.TOTP != nil, passkeys: tcfg.WebAuthn != nil}, nil
}

// Prefix is the route prefix Handler serves under.
func (e *Engine) Prefix() string { return e.prefix }

// Handler serves the library routes under Prefix.
func (e *Engine) Handler() http.Handler { return e.auth.Handler() }

// Auth exposes the instance for guards and tests.
func (e *Engine) Auth() *theauth.TheAuth { return e.auth }

// Close stops the library's background work. It does not close the database.
func (e *Engine) Close() { e.auth.Close() }
