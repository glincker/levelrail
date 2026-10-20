package trafficpolicy

import (
	"fmt"
	"regexp"
	"strings"
)

// Path match kinds.
const (
	MatchPrefix = "prefix"
	MatchExact  = "exact"
	MatchRegex  = "regex"
)

// Match selects requests by path and, optionally, method.
type Match struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	Methods []string `json:"methods,omitempty"`
}

var httpMethods = map[string]bool{
	"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "OPTIONS": true, "CONNECT": true, "TRACE": true,
}

func validateMatch(col *collector, field string, m Match, l Limits, allowedMethods map[string]bool) {
	switch m.Kind {
	case MatchPrefix, MatchExact:
		if !strings.HasPrefix(m.Path, "/") {
			col.add(field+".path", "must start with /")
		}
		if strings.ContainsAny(m.Path, "*?#{} \t") || !SafeHeaderValue(m.Path) {
			col.add(field+".path", "must be a plain path (no wildcards, spaces, braces or query); use regex for patterns")
		}
		if len(m.Path) > l.MaxRegexLen {
			col.add(field+".path", "must be at most %d bytes", l.MaxRegexLen)
		}
	case MatchRegex:
		if m.Path == "" || len(m.Path) > l.MaxRegexLen {
			col.add(field+".path", "must be a regular expression of 1 to %d bytes", l.MaxRegexLen)
		} else if _, err := regexp.Compile(m.Path); err != nil {
			col.add(field+".path", "is not a valid RE2 regular expression: %v", err)
		}
		if strings.ContainsAny(m.Path, "{}") && strings.Contains(m.Path, "{http.") {
			col.add(field+".path", "must not contain placeholders")
		}
	default:
		col.add(field+".kind", "must be prefix, exact or regex")
	}
	if allowedMethods == nil {
		allowedMethods = httpMethods
	}
	for i, method := range m.Methods {
		if !allowedMethods[method] {
			col.add(fmt.Sprintf("%s.methods[%d]", field, i), "%q is not allowed here", method)
		}
	}
}

// Matches reports whether method and path satisfy m. Used by previews and
// the cache handler; the Caddy config uses native matchers instead.
func (m Match) Matches(method, path string) bool {
	if len(m.Methods) > 0 {
		ok := false
		for _, want := range m.Methods {
			if want == method {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	switch m.Kind {
	case MatchExact:
		return path == m.Path
	case MatchPrefix:
		return PrefixMatches(m.Path, path)
	case MatchRegex:
		re, err := regexp.Compile(m.Path)
		return err == nil && re.MatchString(path)
	}
	return false
}

// PrefixMatches is segment aware: /api matches /api and /api/x, not /apix.
func PrefixMatches(prefix, path string) bool {
	if prefix == "/" {
		return true
	}
	p := strings.TrimSuffix(prefix, "/")
	return path == p || strings.HasPrefix(path, p+"/")
}

// Describe renders m for humans, e.g. "GET, HEAD paths under /api".
func (m Match) Describe() string {
	var s string
	switch m.Kind {
	case MatchExact:
		s = "path " + m.Path
	case MatchRegex:
		s = "paths matching " + m.Path
	default:
		s = "paths under " + m.Path
	}
	if len(m.Methods) > 0 {
		s = strings.Join(m.Methods, ", ") + " " + s
	}
	return s
}
