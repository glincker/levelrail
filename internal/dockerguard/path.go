package dockerguard

import (
	"errors"
	"regexp"
	"strings"
)

var (
	errEncodedSeparator = errors.New("encoded path separator")
	errDotSegment       = errors.New("dot segment in path")
	errControlChar      = errors.New("control or backslash character in path")
	errEmptyPath        = errors.New("empty path")
)

var versionSegment = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)

// canonicalPath is a request path reduced to the form the matcher sees and
// the daemon receives, so the two can never disagree.
type canonicalPath struct {
	// Version is the API version prefix ("v1.43") or empty.
	Version string
	// Path is the versionless path, for example /containers/abc/json.
	Path string
}

// Forward is the path sent upstream: the version prefix, when present, kept.
func (c canonicalPath) Forward() string {
	if c.Version == "" {
		return c.Path
	}
	return "/" + c.Version + c.Path
}

// canonicalize rejects encoded separators, dot segments and control
// characters outright instead of resolving them, and collapses repeated
// slashes. escaped is the request's escaped path, decoded its decoded form.
func canonicalize(escaped, decoded string) (canonicalPath, error) {
	lower := strings.ToLower(escaped)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%00") {
		return canonicalPath{}, errEncodedSeparator
	}
	var segs []string
	for _, s := range strings.Split(decoded, "/") {
		switch {
		case s == "":
			continue
		case s == "." || s == "..":
			return canonicalPath{}, errDotSegment
		case strings.ContainsAny(s, "\\\x00\r\n\t"):
			return canonicalPath{}, errControlChar
		}
		segs = append(segs, s)
	}
	var c canonicalPath
	if len(segs) > 0 && versionSegment.MatchString(segs[0]) {
		c.Version, segs = segs[0], segs[1:]
	}
	if len(segs) == 0 {
		return canonicalPath{}, errEmptyPath
	}
	c.Path = "/" + strings.Join(segs, "/")
	return c, nil
}

// matchPattern reports whether path matches pattern. A "*" segment matches
// exactly one segment; "**" matches one or more, so image references with
// slashes (ghcr.io/org/app:tag) fit /images/**/json.
func matchPattern(pattern, path string) bool {
	return matchSegs(splitSegs(pattern), splitSegs(path))
}

func splitSegs(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		switch pat[0] {
		case "**":
			for i := 1; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		case "*":
			if len(segs) == 0 || segs[0] == "" {
				return false
			}
		default:
			if len(segs) == 0 || segs[0] != pat[0] {
				return false
			}
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
