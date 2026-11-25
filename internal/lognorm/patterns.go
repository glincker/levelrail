package lognorm

import (
	"regexp"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// Registered-value masking misses secrets the platform never saw, so common
// credential shapes are masked by pattern as well.
var (
	pemRe       = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`)
	jwtRe       = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]*`)
	authHeadRe  = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+\S+`)
	urlCredRe   = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/:@]+:[^\s/]+@`)
	querySecret = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:token|key|sig|signature|password|secret)=)[^&\s"']+`)
)

// MaskPatterns replaces credential-shaped text with the secretmask marker.
func MaskPatterns(s string) string {
	s = pemRe.ReplaceAllString(s, secretmask.Marker)
	s = jwtRe.ReplaceAllString(s, secretmask.Marker)
	s = authHeadRe.ReplaceAllString(s, "$1 "+secretmask.Marker)
	s = urlCredRe.ReplaceAllString(s, "${1}"+secretmask.Marker+"@")
	return querySecret.ReplaceAllString(s, "${1}"+secretmask.Marker)
}

// Scrub applies the process-wide registry and then the pattern pass, for text
// that is about to leave the process.
func Scrub(s string) string {
	return MaskPatterns(secretmask.Mask(s))
}
