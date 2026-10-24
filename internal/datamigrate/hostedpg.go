package datamigrate

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Env names for the hosted Postgres move, in the style of the migration hub.
const (
	// EnvHostedAllowInsecure lets a public host be reached without TLS.
	EnvHostedAllowInsecure = "APP_MIGRATE_HOSTED_ALLOW_INSECURE"
	// EnvHostedMaxBytes is the size above which a dump and restore is not
	// recommended and needs an explicit override.
	EnvHostedMaxBytes = "APP_MIGRATE_HOSTED_MAX_BYTES"
)

// DefaultHostedMaxBytes is Heroku's own dump and restore ceiling (about 100 GB),
// used only when EnvHostedMaxBytes is unset.
const DefaultHostedMaxBytes int64 = 100 << 30

// HostedConfig is the operator policy for hosted Postgres sources.
type HostedConfig struct {
	AllowInsecure bool
	MaxBytes      int64
}

// HostedConfigFromEnv reads the policy through getenv (os.Getenv in production).
func HostedConfigFromEnv(getenv func(string) string) (HostedConfig, error) {
	cfg := HostedConfig{MaxBytes: DefaultHostedMaxBytes}
	if v := strings.TrimSpace(getenv(EnvHostedAllowInsecure)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("%s must be true or false", EnvHostedAllowInsecure)
		}
		cfg.AllowInsecure = b
	}
	if v := strings.TrimSpace(getenv(EnvHostedMaxBytes)); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("%s must be a positive number of bytes", EnvHostedMaxBytes)
		}
		cfg.MaxBytes = n
	}
	return cfg, nil
}

// HostedSource is a validated postgres:// URL, split into a Source. Obtain the
// URL from the provider, for Heroku `heroku config:get DATABASE_URL -a <app>`.
type HostedSource struct {
	Source Source
	// Redacted is the URL with the password masked, safe to log and store.
	Redacted string
	// Private is true when the host is a loopback, private or internal name.
	Private bool
	raw     string
}

// String and GoString keep the credentials out of fmt output.
func (h HostedSource) String() string { return h.Redacted }

// GoString is String, so %#v is redacted too.
func (h HostedSource) GoString() string { return h.Redacted }

// Scrub removes the URL and its password from any text, for errors and plans.
func (h HostedSource) Scrub(text string) string {
	if h.raw != "" {
		text = strings.ReplaceAll(text, h.raw, h.Redacted)
	}
	return Redact(h.Source.Scrub(text))
}

// SecretURL is a connection URL that prints, logs and marshals redacted, so a
// plan holding it cannot leak the password by accident.
type SecretURL string

// String is the redacted URL.
func (u SecretURL) String() string { return RedactURL(string(u)) }

// GoString is String, so %#v is redacted too.
func (u SecretURL) GoString() string { return u.String() }

// MarshalText stores only the redacted form.
func (u SecretURL) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// RedactURL masks the password of a connection URL without parsing it, so a
// malformed URL cannot make an error echo the secret.
func RedactURL(raw string) string {
	if !strings.Contains(raw, "://") {
		return Redact(raw)
	}
	scheme, rest, _ := strings.Cut(raw, "://")
	authority, tail := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority, tail = rest[:i], rest[i:]
	}
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		user := authority[:at]
		if c := strings.Index(user, ":"); c >= 0 {
			user = user[:c] + ":***"
		}
		authority = user + authority[at:]
	}
	return scheme + "://" + authority + tail
}

var allowedURLParams = map[string]bool{"sslmode": true, "connect_timeout": true, "application_name": true}

// hostedSSLModes are the libpq modes that encrypt the connection.
var hostedSSLModes = map[string]bool{"require": true, "verify-ca": true, "verify-full": true}

// ParseHostedURL validates raw and splits it. Errors never echo the URL.
// A public host must use TLS: a missing sslmode becomes require, and an
// explicit disable, allow or prefer is refused unless cfg.AllowInsecure.
func ParseHostedURL(raw string, cfg HostedConfig) (HostedSource, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return HostedSource{}, errors.New("the database URL is not valid, expected postgres://user:password@host:port/database")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return HostedSource{}, errors.New("the database URL must start with postgres:// or postgresql://")
	}
	if u.User == nil || u.User.Username() == "" {
		return HostedSource{}, errors.New("the database URL must include a user")
	}
	pass, _ := u.User.Password()
	port := 0
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			return HostedSource{}, errors.New("the database URL has an invalid port")
		}
	}
	src := Source{
		Host: u.Hostname(), Port: port, User: u.User.Username(), Password: pass,
		Database: strings.TrimPrefix(u.Path, "/"),
	}
	if err := src.Validate(EnginePostgres); err != nil {
		return HostedSource{}, fmt.Errorf("the database URL is not usable: %w", err)
	}
	q := u.Query()
	for k := range q {
		if !allowedURLParams[k] {
			return HostedSource{}, fmt.Errorf("the database URL parameter %q is not supported", k)
		}
	}
	mode := q.Get("sslmode")
	private := isPrivateHost(src.Host)
	switch {
	case mode == "" && !private:
		mode = "require"
	case mode != "" && !hostedSSLModes[mode] && !map[string]bool{"disable": true, "allow": true, "prefer": true}[mode]:
		return HostedSource{}, errors.New("the database URL has an unknown sslmode")
	case mode != "" && !hostedSSLModes[mode] && !private && !cfg.AllowInsecure:
		return HostedSource{}, fmt.Errorf("sslmode=%s sends the data unencrypted over the internet: use sslmode=require or verify-full (or set %s on a trusted network)", mode, EnvHostedAllowInsecure)
	}
	if hostedSSLModes[mode] {
		src.TLS, src.SSLMode = true, mode
	}
	hs := HostedSource{Source: src, Private: private, raw: raw}
	hs.Redacted = RedactURL(raw)
	return hs, nil
}

// isPrivateHost reports loopback, private-range and internal names.
func isPrivateHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if addr, err := netip.ParseAddr(h); err == nil {
		return addr.IsLoopback() || addr.IsPrivate()
	}
	if !strings.Contains(h, ".") {
		return true
	}
	for _, suffix := range []string{".internal", ".local", ".lan", ".svc", ".cluster.local"} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}
