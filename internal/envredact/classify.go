// Package envredact decides which environment values are sensitive and
// masks them for every read surface. Consumers that need the real value
// (deploy, reconcile, step-up reveal) read the store directly.
package envredact

import (
	"net/url"
	"strings"
	"unicode"
)

// Placeholder stands in for a sensitive value on every read surface. A
// write that sends it back for an existing key keeps the stored value.
const Placeholder = "••••••••"

// Reasons a value was classified sensitive or plain.
const (
	ReasonName     = "name"
	ReasonValue    = "value"
	ReasonOverride = "override"
	ReasonPublic   = "public"
)

const (
	previewMinLen  = 16
	previewSideLen = 4
	minTokenLen    = 32
)

var sensitiveSegments = map[string]bool{
	"KEY": true, "SECRET": true, "TOKEN": true, "PASSWORD": true, "PASSWD": true,
	"PWD": true, "PASS": true, "PRIVATE": true, "CREDENTIAL": true, "CREDENTIALS": true,
	"DSN": true, "AUTH": true, "SIGNING": true, "ENCRYPTION": true, "SALT": true,
	"PEPPER": true, "APIKEY": true, "SECRETKEY": true, "ACCESSKEY": true, "CONNSTR": true,
}

var sensitiveSubstrings = []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE", "DATABASE_URL", "APIKEY", "API_KEY"}

// publicPrefixes are framework prefixes compiled into browser bundles, so
// their values are public by construction (publishable keys and URLs).
var publicPrefixes = []string{"NEXT_PUBLIC_", "VITE_", "REACT_APP_", "PUBLIC_", "NUXT_PUBLIC_", "EXPO_PUBLIC_", "GATSBY_"}

// valueHeuristicSkip marks names whose long hex values are identifiers,
// not credentials (commit SHAs, image digests, build versions).
var valueHeuristicSkip = map[string]bool{"SHA": true, "COMMIT": true, "REVISION": true, "VERSION": true, "DIGEST": true, "BUILD": true, "RELEASE": true}

var knownTokenPrefixes = []string{"sk_live_", "sk_test_", "rk_live_", "ghp_", "gho_", "ghs_", "ghu_", "github_pat_", "glpat-", "xoxb-", "xoxp-", "xoxa-", "xapp-", "AKIA", "ASIA", "AIza", "SG.", "npm_", "hf_", "sk-"}

func segments(key string) []string {
	return strings.FieldsFunc(strings.ToUpper(key), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// IsPublicName reports whether key carries a browser-public framework prefix.
func IsPublicName(key string) bool {
	up := strings.ToUpper(key)
	for _, p := range publicPrefixes {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}

// SensitiveName reports whether key's name alone marks it sensitive.
func SensitiveName(key string) bool {
	up := strings.ToUpper(key)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(up, s) {
			return true
		}
	}
	for _, seg := range segments(key) {
		if sensitiveSegments[seg] {
			return true
		}
	}
	return false
}

// SensitiveValue reports whether value looks like key material on its own.
func SensitiveValue(key, value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	if strings.Contains(v, "-----BEGIN ") {
		return true
	}
	if urlHasPassword(v) {
		return true
	}
	if looksLikeJWT(v) {
		return true
	}
	for _, p := range knownTokenPrefixes {
		if strings.HasPrefix(v, p) && len(v) >= len(p)+16 && !strings.ContainsAny(v, " \t") {
			return true
		}
	}
	for _, seg := range segments(key) {
		if valueHeuristicSkip[seg] {
			return false
		}
	}
	return looksLikeHex(v) || looksLikeBase64(v)
}

func urlHasPassword(v string) bool {
	if !strings.Contains(v, "://") || !strings.Contains(v, "@") {
		return false
	}
	u, err := url.Parse(v)
	if err != nil || u.User == nil {
		return false
	}
	p, ok := u.User.Password()
	return ok && p != ""
}

func looksLikeJWT(v string) bool {
	if !strings.HasPrefix(v, "eyJ") {
		return false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts[:2] {
		if len(p) < 4 || !allIn(p, isBase64URLRune) {
			return false
		}
	}
	return true
}

func looksLikeHex(v string) bool {
	if len(v) < minTokenLen || !allIn(v, isHexRune) {
		return false
	}
	return strings.ContainsAny(v, "0123456789") && strings.ContainsAny(strings.ToLower(v), "abcdef")
}

func looksLikeBase64(v string) bool {
	if len(v) < minTokenLen || !allIn(v, isBase64AnyRune) {
		return false
	}
	var upper, lower, digit bool
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	return upper && lower && digit
}

func allIn(s string, ok func(rune) bool) bool {
	for _, r := range s {
		if !ok(r) {
			return false
		}
	}
	return true
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isBase64URLRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' || r == '='
}

func isBase64AnyRune(r rune) bool {
	return isBase64URLRune(r) || r == '+' || r == '/'
}

// Classify reports whether key=value is sensitive and why. override, when
// non-nil, is the operator's explicit choice and always wins.
func Classify(key, value string, override *bool) (sensitive bool, reason string) {
	if override != nil {
		return *override, ReasonOverride
	}
	if IsPublicName(key) {
		return false, ReasonPublic
	}
	if SensitiveName(key) {
		return true, ReasonName
	}
	if SensitiveValue(key, value) {
		return true, ReasonValue
	}
	return false, ""
}
