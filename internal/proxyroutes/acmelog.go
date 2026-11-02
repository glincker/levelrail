package proxyroutes

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// retriedAt remembers when each domain was last nudged, so a failure logged
// before the retry is not shown as the current error.
var retriedAt sync.Map

// RecordRetry marks now as the moment a retry was requested for domain.
func RecordRetry(domain string, at time.Time) { retriedAt.Store(strings.ToLower(domain), at) }

// RetriedAt returns the last recorded retry time, zero when none.
func RetriedAt(domain string) time.Time {
	v, ok := retriedAt.Load(strings.ToLower(domain))
	if !ok {
		return time.Time{}
	}
	t, _ := v.(time.Time)
	return t
}

// logTime matches the RFC 3339 stamp Traefik writes as time="..." or "time":"...".
var logTime = regexp.MustCompile(`"?time"?[=:]\s*"?(\d{4}-\d{2}-\d{2}T[0-9:.]+(?:Z|[+-]\d{2}:\d{2}))`)

func lineTime(l string) (time.Time, bool) {
	m := logTime.FindStringSubmatch(l)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, m[1])
	return t, err == nil
}

// maxACMELogLine bounds the log line shown to the operator.
const maxACMELogLine = 300

var acmeFailureMarkers = []string{
	"unable to obtain acme certificate",
	"acme: error",
	"error obtaining certificate",
	"unable to generate a certificate",
	"ratelimited",
	"too many certificates",
	"acme: cannot",
}

// ACMEFailure returns the most recent proxy log line that reports a failed
// certificate request for domain. Lines are matched case-insensitively and
// the result is cut to a readable length; callers mask secrets before storing.
func ACMEFailure(logs, domain string) string {
	return ACMEFailureSince(logs, domain, time.Time{})
}

// ACMEFailureSince is ACMEFailure ignoring lines stamped before since; lines
// without a readable timestamp are kept because their age is unknown.
func ACMEFailureSince(logs, domain string, since time.Time) string {
	d := strings.ToLower(domain)
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.ToLower(lines[i])
		if !strings.Contains(l, d) {
			continue
		}
		if t, ok := lineTime(lines[i]); ok && t.Before(since) {
			continue
		}
		for _, m := range acmeFailureMarkers {
			if strings.Contains(l, m) {
				return trimLogLine(lines[i])
			}
		}
	}
	return ""
}

func trimLogLine(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxACMELogLine {
		return s
	}
	cut := maxACMELogLine
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// IsACMERateLimit reports whether a failure line is a CA rate limit.
func IsACMERateLimit(line string) bool {
	l := strings.ToLower(line)
	return strings.Contains(l, "ratelimited") || strings.Contains(l, "too many certificates") || strings.Contains(l, "rate limit")
}
