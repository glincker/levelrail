package proxyroutes

import (
	"strings"
	"unicode/utf8"
)

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
	d := strings.ToLower(domain)
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.ToLower(lines[i])
		if !strings.Contains(l, d) {
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
