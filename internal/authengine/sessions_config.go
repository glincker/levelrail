package authengine

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

// Environment variables for the sessions area. Every default matches the
// behavior of the legacy login limiter and session store.
const (
	// EnvSessionTTL is the absolute session lifetime (Go duration), shared with the legacy store.
	EnvSessionTTL = "APP_SESSION_TTL"
	// EnvSessionIdleTimeout expires a session unused for this long; 0 disables it.
	EnvSessionIdleTimeout = "APP_AUTH_ENGINE_SESSION_IDLE_TIMEOUT"
	// EnvSessionLinkTTL is how long a minted session link can be exchanged.
	EnvSessionLinkTTL = "APP_AUTH_ENGINE_SESSION_LINK_TTL"
	// EnvLoginGraceFailures is the failed logins per (IP, username) before backoff starts.
	EnvLoginGraceFailures = "APP_AUTH_ENGINE_LOGIN_GRACE_FAILURES"
	// EnvLoginBaseDelay is the first backoff delay, doubled per further failure.
	EnvLoginBaseDelay = "APP_AUTH_ENGINE_LOGIN_BASE_DELAY"
	// EnvLoginMaxDelay caps the backoff delay.
	EnvLoginMaxDelay = "APP_AUTH_ENGINE_LOGIN_MAX_DELAY"
	// EnvLoginResetAfter forgets failure counts after this much idle time.
	EnvLoginResetAfter = "APP_AUTH_ENGINE_LOGIN_RESET_AFTER"
	// EnvLoginUserMaxFailures is the consecutive failures against one user, from any IP, that lock the account.
	EnvLoginUserMaxFailures = "APP_AUTH_ENGINE_LOGIN_USER_MAX_FAILURES"
	// EnvLoginUserLockout is how long a per-user lockout lasts.
	EnvLoginUserLockout = "APP_AUTH_ENGINE_LOGIN_USER_LOCKOUT"
	// EnvRateLimitPerEmail is the per-username credential requests allowed per minute.
	EnvRateLimitPerEmail = "APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL"
	// EnvStreamWatchInterval is how often a live stream re-validates its session.
	EnvStreamWatchInterval = "APP_AUTH_ENGINE_STREAM_WATCH_INTERVAL"

	defaultSessionTTL        = 24 * time.Hour
	defaultSessionLinkTTL    = 2 * time.Minute
	defaultLoginGrace        = 3
	defaultLoginBaseDelay    = time.Second
	defaultLoginMaxDelay     = 15 * time.Minute
	defaultLoginResetAfter   = 15 * time.Minute
	defaultUserMaxFailures   = 10
	defaultUserLockout       = 15 * time.Minute
	defaultRatePerMinute     = 60
	defaultStreamWatchPeriod = 5 * time.Second
)

// SessionsHooks are the callbacks the sessions area needs from its host.
type SessionsHooks struct {
	// Audit receives one record per auth event; nil drops them.
	Audit func(ctx context.Context, rec AuditRecord)
	// Mail relays library emails (password reset) through the host's mailer; nil drops them.
	Mail *MailRelay
}

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return def
	}
	return d
}

func envPositiveInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// StreamWatchInterval is how often a live stream re-validates its session.
func StreamWatchInterval() time.Duration {
	if d := envDuration(EnvStreamWatchInterval, defaultStreamWatchPeriod); d > 0 {
		return d
	}
	return defaultStreamWatchPeriod
}

func sessionLinkTTL() time.Duration {
	if d := envDuration(EnvSessionLinkTTL, defaultSessionLinkTTL); d > 0 {
		return d
	}
	return defaultSessionLinkTTL
}

// applySessionsConfig layers the sessions-area settings onto tcfg. It only
// runs when the area is active, so legacy behavior is untouched.
func applySessionsConfig(tcfg *theauth.Config, s *Sessions, throttle theauth.LoginThrottleStore) {
	ttl := envDuration(EnvSessionTTL, defaultSessionTTL)
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	tcfg.SessionTTL = ttl
	tcfg.SessionIdleTimeout = envDuration(EnvSessionIdleTimeout, 0)
	if tcfg.SessionIdleTimeout > 0 && tcfg.SessionTouchInterval <= 0 {
		tcfg.SessionTouchInterval = tcfg.SessionIdleTimeout / 4
	}
	tcfg.SessionLinks = &theauth.SessionLinksConfig{
		TTL:               sessionLinkTTL(),
		CredentialChecker: theauth.CredentialCheckerFunc(s.checkCredential),
	}
	tcfg.LoginThrottle = &theauth.LoginThrottleConfig{
		Store:           throttle,
		GraceFailures:   envPositiveInt(EnvLoginGraceFailures, defaultLoginGrace),
		BaseDelay:       envDuration(EnvLoginBaseDelay, defaultLoginBaseDelay),
		MaxDelay:        envDuration(EnvLoginMaxDelay, defaultLoginMaxDelay),
		ResetAfter:      envDuration(EnvLoginResetAfter, defaultLoginResetAfter),
		UserMaxFailures: envPositiveInt(EnvLoginUserMaxFailures, defaultUserMaxFailures),
		UserLockout:     envDuration(EnvLoginUserLockout, defaultUserLockout),
	}
	if tcfg.RateLimitPerIP == 0 {
		tcfg.RateLimitPerIP = defaultRatePerMinute
	}
	tcfg.RateLimitPerEmail = envPositiveInt(EnvRateLimitPerEmail, defaultRatePerMinute)
	if tcfg.Bootstrap == nil {
		tcfg.Bootstrap = &theauth.BootstrapConfig{SuppressSetupTokenLog: true}
	}
	if s.hooks.Mail != nil {
		tcfg.EmailSender = s.hooks.Mail
	}
	tcfg.AuthEventSink = ChainAuthEventSinks(tcfg.AuthEventSink, s.eventSink)
}

// ChainAuthEventSinks returns a sink that calls every non-nil sink in order.
func ChainAuthEventSinks(sinks ...theauth.AuthEventSink) theauth.AuthEventSink {
	var live []theauth.AuthEventSink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	}
	return func(ctx context.Context, e theauth.AuthEvent) {
		for _, s := range live {
			s(ctx, e)
		}
	}
}
