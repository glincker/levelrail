package datamigrate

import (
	"fmt"
	"regexp"
	"strings"
)

// Failure codes returned by Humanize.
const (
	FailureTimeout    = "source_timeout"
	FailureRefused    = "source_refused"
	FailureDNS        = "source_dns"
	FailureAuth       = "auth_failed"
	FailureNoDatabase = "database_missing"
	FailureSSL        = "ssl_required"
	FailurePermission = "permission_denied"
	FailureDisk       = "out_of_disk"
	FailureNotRunning = "target_not_running"
	FailureMismatch   = "verify_mismatch"
	FailureUnknown    = "unknown"
)

const maxTechnicalLen = 2000

// FailureInfo is a copy failure in words an operator can act on. Technical
// holds the redacted raw error, to be shown only on request.
type FailureInfo struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Hint      string `json:"hint"`
	Technical string `json:"technical,omitempty"`
}

var (
	serverAtRe  = regexp.MustCompile(`(?i)server at "?([^",\s]+)"?,? port (\d+)`)
	hostPortRe  = regexp.MustCompile(`(?i)(?:connect to|dial tcp|at)\s+([0-9a-z.\-]+):(\d{2,5})`)
	dbMissingRe = regexp.MustCompile(`(?i)database "([^"]+)" does not exist`)

	envSecretRe  = regexp.MustCompile(`(?i)\b(SRC_PASSWORD|PGPASSWORD|MYSQL_PWD|REDISCLI_AUTH|PASSWORD|PASSWD|SECRET|TOKEN)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)
	uriSecretRe  = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://[^\s:/@]*:)[^\s@/]+(@)`)
	flagSecretRe = regexp.MustCompile(`(\s-p)\S+`)
)

// Redact removes passwords and credentials embedded in connection strings
// from raw error text.
func Redact(raw string) string {
	out := envSecretRe.ReplaceAllString(raw, "${1}${2}***")
	out = uriSecretRe.ReplaceAllString(out, "${1}***${2}")
	return flagSecretRe.ReplaceAllString(out, "${1}***")
}

func clip(s string) string {
	if len(s) <= maxTechnicalLen {
		return s
	}
	return s[:maxTechnicalLen] + "..."
}

func endpoint(raw, fallbackHost string, fallbackPort int) string {
	if m := serverAtRe.FindStringSubmatch(raw); m != nil {
		return m[1] + ":" + m[2]
	}
	if m := hostPortRe.FindStringSubmatch(raw); m != nil {
		return m[1] + ":" + m[2]
	}
	switch {
	case fallbackHost != "" && fallbackPort > 0:
		return fmt.Sprintf("%s:%d", fallbackHost, fallbackPort)
	case fallbackHost != "":
		return fallbackHost
	}
	return "the source"
}

// Humanize maps a raw copy error to a short sentence with a fix hint. The
// fallback host and port come from the recorded copy and are used when the
// error text does not name them.
func Humanize(raw, fallbackHost string, fallbackPort int) FailureInfo {
	low := strings.ToLower(raw)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(low, s) {
				return true
			}
		}
		return false
	}
	ep := endpoint(raw, fallbackHost, fallbackPort)
	info := FailureInfo{Technical: clip(Redact(raw))}
	switch {
	case has("not running yet"):
		info.Code = FailureNotRunning
		info.Summary = "The managed database was not running when the copy started."
		info.Hint = "Wait for the database to become ready, then retry the copy."
	case has("tables differ"):
		info.Code = FailureMismatch
		info.Summary = "The copy finished, but some tables do not match the source."
		info.Hint = "Retry the copy. If it keeps happening, stop writes to the source while it runs."
	case has("no space left on device", "disk full", "could not extend file"):
		info.Code = FailureDisk
		info.Summary = "This server ran out of disk space while copying."
		info.Hint = "Free space or grow the disk, then retry. The copy replaces the target's contents, so repeating it is safe."
	case has("no encryption", "require_secure_transport", "ssl connection is required", "server does not support ssl", "ssl is required", "sslmode"):
		info.Code = FailureSSL
		info.Summary = fmt.Sprintf("The source database at %s requires or refuses an encrypted connection.", ep)
		info.Hint = "Check the source's SSL setting and the SSL mode used for the copy, then retry."
	case has("password authentication failed", "authentication failed", "access denied for user", "auth failed", "wrongpass", "noauth", "invalid password"):
		info.Code = FailureAuth
		info.Summary = fmt.Sprintf("The source database at %s rejected the username or password.", ep)
		info.Hint = "Re-enter the source credentials and retry. The password is never stored."
	case dbMissingRe.MatchString(raw) || has("unknown database"):
		name := "the requested database"
		if m := dbMissingRe.FindStringSubmatch(raw); m != nil {
			name = `"` + m[1] + `"`
		}
		info.Code = FailureNoDatabase
		info.Summary = fmt.Sprintf("The source at %s has no database called %s.", ep, name)
		info.Hint = "Check the source database name and retry."
	case has("permission denied", "must be owner", "insufficient privilege", "command denied", "noperm"):
		info.Code = FailurePermission
		info.Summary = fmt.Sprintf("The source user on %s is not allowed to read everything the copy needs.", ep)
		info.Hint = "Use a source role that can read all tables (a superuser or the database owner) and retry."
	case has("could not translate host name", "no such host", "name or service not known", "temporary failure in name resolution"):
		info.Code = FailureDNS
		info.Summary = "The source host name could not be resolved."
		info.Hint = "Check the host name for typos, or use the server's IP address, then retry."
	case has("connection refused"):
		info.Code = FailureRefused
		info.Summary = fmt.Sprintf("Nothing is listening on the source at %s, or it refused the connection.", ep)
		info.Hint = "Check the port and that the database accepts remote connections from this server, then retry."
	case has("timeout expired", "timed out", "i/o timeout", "no route to host", "network is unreachable", "deadline exceeded"):
		info.Code = FailureTimeout
		info.Summary = fmt.Sprintf("The source database did not answer on %s in time.", ep)
		info.Hint = "Check that it is reachable from this server (firewall, allowed IPs) and retry."
	default:
		info.Code = FailureUnknown
		info.Summary = "The data copy failed."
		info.Hint = "Open the technical details for the exact error, fix the cause and retry."
	}
	return info
}
