package ingress

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
)

// fallbackKeyInternal is certmagic's issuer key for Caddy's internal issuer.
const fallbackKeyInternal = "local"

// acmeUnissuableSuffixes are TLDs a public CA never signs (RFC 2606, RFC 6761
// and Let's Encrypt's own policy), so ACME for them can only fail.
var acmeUnissuableSuffixes = []string{
	"localhost", "test", "example", "invalid", "local", "internal",
}

// acmeUnissuableDomains are second-level names RFC 2606 reserves.
var acmeUnissuableDomains = []string{"example.com", "example.net", "example.org"}

// ACMEIssuable reports whether a public CA can plausibly issue for host.
// Wildcards are judged by their base name.
func ACMEIssuable(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "*."), "."))
	if h == "" || net.ParseIP(h) != nil || !strings.Contains(h, ".") {
		return false
	}
	return !reservedName(h)
}

func reservedName(h string) bool {
	for _, s := range acmeUnissuableSuffixes {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	for _, d := range acmeUnissuableDomains {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// Reasons ValidateACMEContact rejects an address, so callers can tell them apart.
var (
	ErrACMEContactInvalid  = errors.New("acme_email is not a valid email address")
	ErrACMEContactReserved = errors.New("acme_email uses a domain Let's Encrypt rejects (example.com, .test, .invalid, .localhost and similar); use a real, reachable address")
)

// ValidateACMEContact checks the ACME account contact address a CA will accept.
func ValidateACMEContact(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return ErrACMEContactInvalid
	}
	at := strings.LastIndex(email, "@")
	domain := strings.ToLower(email[at+1:])
	if at <= 0 || !strings.Contains(domain, ".") || net.ParseIP(strings.Trim(domain, "[]")) != nil {
		return ErrACMEContactInvalid
	}
	if reservedName(domain) {
		return ErrACMEContactReserved
	}
	return nil
}

// withInternalFallback puts the internal issuer behind primary, so a failed
// public issuance serves a locally signed certificate instead of none.
func withInternalFallback(primary any, lifetime string) []any {
	return []any{primary, InternalIssuer{Module: "internal", Lifetime: lifetime}}
}

// LoggingConfig is Caddy's logging section, used only to tap ACME errors.
type LoggingConfig struct {
	Logs map[string]*LogConfig `json:"logs"`
}

// LogConfig is one named Caddy log.
type LogConfig struct {
	Writer  map[string]string `json:"writer"`
	Encoder map[string]string `json:"encoder"`
	Level   string            `json:"level,omitempty"`
	Include []string          `json:"include"`
}

func newACMELogTap() *LoggingConfig {
	return &LoggingConfig{Logs: map[string]*LogConfig{
		"acme_tap": {
			Writer:  map[string]string{"output": "acme_tap"},
			Encoder: map[string]string{"format": "json"},
			Level:   "ERROR",
			Include: []string{"tls.obtain", "tls.renew"},
		},
	}}
}

func init() {
	caddy.RegisterModule(acmeLogTap{})
}

// acmeLogTap is a Caddy log writer that records each failed public issuance
// attempt, which certmagic only logs (never as cert_failed) when a fallback
// issuer then succeeds. Lines are passed through to stderr unchanged.
type acmeLogTap struct{}

func (acmeLogTap) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.logging.writers.acme_tap",
		New: func() caddy.Module { return new(acmeLogTap) },
	}
}

func (acmeLogTap) String() string                      { return "acme_tap" }
func (acmeLogTap) WriterKey() string                   { return "acme_tap" }
func (acmeLogTap) OpenWriter() (io.WriteCloser, error) { return tapWriter{}, nil }

type tapWriter struct{}

func (tapWriter) Close() error { return nil }

func (tapWriter) Write(p []byte) (int, error) {
	recordACMELogLine(p)
	return os.Stderr.Write(p)
}

// recordACMELogLine files a certmagic "could not get certificate" line from a
// non-internal issuer as that identifier's latest failure.
func recordACMELogLine(line []byte) {
	var l struct {
		Msg        string `json:"msg"`
		Identifier string `json:"identifier"`
		Issuer     string `json:"issuer"`
		Error      string `json:"error"`
	}
	if err := json.Unmarshal(line, &l); err != nil {
		return
	}
	if l.Msg != "could not get certificate from issuer" || l.Identifier == "" || l.Issuer == fallbackKeyInternal {
		return
	}
	defaultACMEFailures.Record(ACMEFailure{
		Identifier: strings.ToLower(l.Identifier),
		Error:      l.Error,
		Renewal:    false,
		At:         time.Now(),
	})
}
