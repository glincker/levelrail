package cloudconn

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ocisign"
)

// AuthOCISession is the browser login (oci session authenticate) method. It lives here so
// the shared constant block stays untouched.
const AuthOCISession = "oci_session"

// ClassNeedsRelogin is reported when the session token is expired or about to be.
const ClassNeedsRelogin = "needs_relogin"

// ErrSessionExpired marks a session token past its exp or inside the relogin window.
var ErrSessionExpired = errors.New("cloudconn: oracle session expired")

// OracleReloginCommand is the exact command that mints a fresh session.
func OracleReloginCommand(profile string) string {
	if profile == "" {
		profile = oracleProfile
	}
	return "oci session authenticate --profile-name " + profile + " --region <region>"
}

// oracleSessionMinRemaining is how long before exp a session counts as needing relogin.
func oracleSessionMinRemaining() time.Duration {
	return time.Duration(envIntDefault("APP_CLOUD_OCI_SESSION_MIN_SECONDS", 300)) * time.Second
}

// OracleSessionExpiry reads exp from the token's JWT payload without verifying it: the
// server only needs the clock, OCI does the real verification.
func OracleSessionExpiry(token string) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || len(token) > oracleMaxSecretBytes() {
		return time.Time{}, invalid("session token is not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, invalid("session token payload is not valid")
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, invalid("session token has no exp")
	}
	return time.Unix(claims.Exp, 0).UTC(), nil
}

// OracleSessionNeedsRelogin is true when exp has passed or is closer than the window.
func OracleSessionNeedsRelogin(exp, now time.Time) bool {
	return !exp.After(now.Add(oracleSessionMinRemaining()))
}

// ValidateOracleSession checks a session credential offline: settings, the key and the JWT shape.
func ValidateOracleSession(c OracleCredential, now time.Time) (time.Time, error) {
	if err := validateOracleSettings(c.TenancyOCID, c.CompartmentOCID, c.Region); err != nil {
		return time.Time{}, err
	}
	if _, err := parseOracleKey(c.Secret.key); err != nil {
		return time.Time{}, err
	}
	exp, err := OracleSessionExpiry(c.SessionToken)
	if err != nil {
		return time.Time{}, err
	}
	if !exp.After(now) {
		return exp, ErrSessionExpired
	}
	return exp, nil
}

// oracleKeyAndID returns the signing key and keyId for either an API key or a session.
func oracleKeyAndID(c OracleCredential, now time.Time) (key *rsa.PrivateKey, keyID string, err error) {
	if c.SessionToken != "" {
		k, perr := parseOracleKey(c.Secret.key)
		if perr != nil {
			return nil, "", perr
		}
		exp, eerr := OracleSessionExpiry(c.SessionToken)
		if eerr != nil {
			return nil, "", eerr
		}
		if OracleSessionNeedsRelogin(exp, now) {
			return nil, "", ErrSessionExpired
		}
		return k, ocisign.SessionKeyID(c.SessionToken), nil
	}
	k, verr := validateOracleSecret(c.Secret)
	if verr != nil {
		return nil, "", verr
	}
	return k, c.TenancyOCID + "/" + c.Secret.UserOCID + "/" + strings.ToLower(c.Secret.Fingerprint), nil
}
