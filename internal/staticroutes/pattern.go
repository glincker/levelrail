package staticroutes

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const splatName = "splat"

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// pattern is a path pattern with :name segments and a trailing * splat,
// compiled to an RE2 expression so matching is linear in the path length.
type pattern struct {
	re    *regexp.Regexp
	names []string
}

func compilePattern(p string) (*pattern, error) {
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("path %q must start with /", p)
	}
	trimmed := strings.TrimSuffix(p, "/")
	var segs []string
	if trimmed != "" {
		segs = strings.Split(trimmed[1:], "/")
	}
	pt := &pattern{}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("(?is)^")
	for i, seg := range segs {
		switch {
		case seg == "":
			return nil, fmt.Errorf("path %q has an empty segment", p)
		case seg == "*":
			if i != len(segs)-1 {
				return nil, fmt.Errorf("path %q: * is only allowed as the last segment", p)
			}
			b.WriteString("(?:/(.*))?")
			pt.names = append(pt.names, splatName)
		case strings.HasPrefix(seg, ":"):
			name := seg[1:]
			if !nameRe.MatchString(name) {
				return nil, fmt.Errorf("path %q: placeholder %q must be :name with letters, digits and _", p, seg)
			}
			if name == splatName || seen[name] {
				return nil, fmt.Errorf("path %q: placeholder :%s is reserved or repeated", p, name)
			}
			seen[name] = true
			b.WriteString("/([^/]+)")
			pt.names = append(pt.names, name)
		case strings.ContainsAny(seg, "*:"):
			return nil, fmt.Errorf("path %q: * and : must be a whole segment", p)
		default:
			b.WriteString("/" + regexp.QuoteMeta(seg))
		}
	}
	if len(segs) == 0 {
		b.WriteString("/")
	} else {
		b.WriteString("/?")
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("path %q: %w", p, err)
	}
	pt.re = re
	return pt, nil
}

// match returns the captured values, or nil when path does not match.
func (pt *pattern) match(path string) map[string]string {
	m := pt.re.FindStringSubmatch(path)
	if m == nil {
		return nil
	}
	caps := make(map[string]string, len(pt.names))
	for i, n := range pt.names {
		caps[n] = m[i+1]
	}
	return caps
}

func (pt *pattern) has(name string) bool {
	for _, n := range pt.names {
		if n == name {
			return true
		}
	}
	return false
}

// substitute fills :name placeholders in to. Values are re-escaped, so a
// decoded %0d or backslash in the request cannot reach a Location header.
func substitute(to string, caps map[string]string) string {
	pathPart, queryPart, hasQuery := strings.Cut(to, "?")
	fill := func(s string, query bool) string {
		return placeholderRe.ReplaceAllStringFunc(s, func(tok string) string {
			v, ok := caps[tok[1:]]
			if !ok {
				return tok
			}
			if query {
				return url.QueryEscape(v)
			}
			segs := strings.Split(v, "/")
			for i, sg := range segs {
				segs[i] = url.PathEscape(sg)
			}
			return strings.Join(segs, "/")
		})
	}
	out := fill(pathPart, false)
	if hasQuery {
		out += "?" + fill(queryPart, true)
	}
	return out
}

// guardLocation collapses leading slashes so a path target can never become
// a protocol relative URL to another host.
func guardLocation(loc string) string {
	if strings.HasPrefix(loc, "/") {
		return "/" + strings.TrimLeft(loc, "/\\")
	}
	return loc
}
