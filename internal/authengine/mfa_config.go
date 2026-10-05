package authengine

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

const (
	// EnvWebAuthnRPID overrides the relying party ID derived from the dashboard URL.
	EnvWebAuthnRPID = "APP_AUTH_ENGINE_WEBAUTHN_RP_ID"
	// EnvWebAuthnOrigins adds comma separated origins beside the dashboard URL origin.
	EnvWebAuthnOrigins = "APP_AUTH_ENGINE_WEBAUTHN_ORIGINS"
	// EnvWebAuthnRequireUV makes passkeys demand PIN or biometric verification.
	EnvWebAuthnRequireUV = "APP_AUTH_ENGINE_WEBAUTHN_REQUIRE_UV"
	// EnvWebAuthnCloneWarning is "reject" (default) or "flag".
	EnvWebAuthnCloneWarning = "APP_AUTH_ENGINE_WEBAUTHN_CLONE_WARNING"
	// EnvMFAMaxFailures is the wrong-code count per user that triggers a lockout.
	EnvMFAMaxFailures = "APP_AUTH_ENGINE_MFA_MAX_FAILURES"
	// EnvMFALockout is how long that lockout lasts (Go duration).
	EnvMFALockout = "APP_AUTH_ENGINE_MFA_LOCKOUT"

	defaultMFAMaxFailures = 5
	defaultMFALockout     = 15 * time.Minute
)

// MFAConfig is the TOTP and passkey policy applied when AreaMFA is active.
type MFAConfig struct {
	// DashboardURL is the canonical public URL the RP ID and origin derive from.
	DashboardURL            string
	RPID                    string
	RPOrigins               []string
	RPDisplayName           string
	RequireUserVerification bool
	CloneWarning            theauth.CloneWarningPolicy
	MaxFailures             int
	Lockout                 time.Duration
}

// MFAConfigFromEnv applies the env overrides on top of base and fills defaults.
func MFAConfigFromEnv(base MFAConfig) MFAConfig {
	if v := strings.TrimSpace(os.Getenv(EnvWebAuthnRPID)); v != "" {
		base.RPID = v
	}
	if base.RPID == "" || base.RPOrigins == nil {
		rp, origin := deriveRP(base.DashboardURL)
		if base.RPID == "" {
			base.RPID = rp
		}
		if origin != "" {
			base.RPOrigins = append(base.RPOrigins, origin)
		}
	}
	for _, o := range strings.Split(os.Getenv(EnvWebAuthnOrigins), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			base.RPOrigins = appendUnique(base.RPOrigins, o)
		}
	}
	if v := os.Getenv(EnvWebAuthnRequireUV); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			base.RequireUserVerification = b
		}
	}
	base.CloneWarning = theauth.CloneWarningReject
	if strings.EqualFold(strings.TrimSpace(os.Getenv(EnvWebAuthnCloneWarning)), string(theauth.CloneWarningFlag)) {
		base.CloneWarning = theauth.CloneWarningFlag
	}
	if v := os.Getenv(EnvMFAMaxFailures); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			base.MaxFailures = n
		}
	}
	if v := os.Getenv(EnvMFALockout); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			base.Lockout = d
		}
	}
	if base.MaxFailures == 0 {
		base.MaxFailures = defaultMFAMaxFailures
	}
	if base.Lockout == 0 {
		base.Lockout = defaultMFALockout
	}
	return base
}

// deriveRP returns the RP ID (host, no port) and origin of the dashboard URL.
// IP hosts yield no RP ID: browsers refuse WebAuthn there.
func deriveRP(dashboardURL string) (rpID, origin string) {
	u, err := url.Parse(strings.TrimSpace(dashboardURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", ""
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return "", ""
	}
	return host, u.Scheme + "://" + u.Host
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// applyMFA mutates tcfg for AreaMFA: WebAuthn relying party and MFA lockout policy.
func applyMFA(tcfg *theauth.Config, cfg Config) {
	if !AreaActive(AreaMFA) {
		return
	}
	m := MFAConfigFromEnv(cfg.MFA)
	if m.RPDisplayName == "" {
		m.RPDisplayName = cfg.TOTPIssuer
	}
	if m.RPID != "" && len(m.RPOrigins) > 0 {
		tcfg.WebAuthn = &theauth.WebAuthnConfig{
			RPID:                    m.RPID,
			RPDisplayName:           m.RPDisplayName,
			RPOrigins:               m.RPOrigins,
			RequireUserVerification: m.RequireUserVerification,
			CloneWarning:            m.CloneWarning,
			UserHandleResolver:      cfg.Directory.ResolveLegacyUserHandle,
		}
	}
	if tcfg.LoginThrottle == nil {
		tcfg.LoginThrottle = &theauth.LoginThrottleConfig{MFAMaxFailures: m.MaxFailures, MFALockout: m.Lockout}
	}
}

// LoadDashboardURL reads the operator's public dashboard URL, or "" when unset.
func LoadDashboardURL(ctx context.Context, db *sql.DB) string {
	var u sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT dashboard_url FROM ingress_settings WHERE id = 1`).Scan(&u); err != nil {
		return ""
	}
	return u.String
}
