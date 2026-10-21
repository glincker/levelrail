package staticroutes

import (
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

type pattern = spec.StaticPattern

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
