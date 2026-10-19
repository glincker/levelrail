package cliconsole

import (
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

const (
	redacted          = envredact.Marker
	maxAuditLineBytes = 512
)

var secretWords = []string{"password", "passwd", "secret", "token", "key", "credential", "passphrase", "hmac", "private", "auth", "cookie", "dsn", "pem", "cert"}

// secretCommandWords mark commands whose positional values are themselves secret.
var secretCommandWords = []string{"secret", "env", "vault", "password", "credential", "users"}

func secretish(name string) bool {
	n := strings.ToLower(name)
	for _, w := range secretWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// Redact renders argv for the audit log with secret-looking values masked.
// It errs toward masking: an audit row never needs the value itself.
func Redact(argv []string, path string) string {
	secretCmd := false
	for _, w := range secretCommandWords {
		if strings.Contains(path, w) {
			secretCmd = true
		}
	}
	pathWords := len(strings.Fields(path))
	out := make([]string, 0, len(argv))
	maskNext := false
	positional := 0
	for i, a := range argv {
		switch {
		case maskNext:
			out = append(out, redacted)
			maskNext = false
			continue
		case i < pathWords && !strings.HasPrefix(a, "-"):
			out = append(out, a)
			continue
		}
		if name, isFlag := flagName(a); isFlag {
			if k, _, inline := strings.Cut(a, "="); inline {
				if secretish(name) {
					out = append(out, k+"="+redacted)
				} else {
					out = append(out, redactValue(a))
				}
				continue
			}
			maskNext = secretish(name)
			out = append(out, a)
			continue
		}
		positional++
		if secretCmd && positional > 1 {
			out = append(out, redacted)
			continue
		}
		out = append(out, redactValue(a))
	}
	line := strings.Join(out, " ")
	if len(line) > maxAuditLineBytes {
		line = line[:maxAuditLineBytes] + "..."
	}
	return line
}

// redactValue masks KEY=VALUE pairs with a secret-looking key and URL userinfo.
func redactValue(a string) string {
	if k, _, ok := strings.Cut(a, "="); ok && secretish(k) {
		return k + "=" + redacted
	}
	if strings.Contains(a, "://") && strings.Contains(a, "@") {
		if u, err := url.Parse(a); err == nil && u.User != nil {
			u.User = url.User(redacted)
			return u.String()
		}
		return redacted
	}
	return a
}
