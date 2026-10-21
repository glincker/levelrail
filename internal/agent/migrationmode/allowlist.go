// Package migrationmode is the read-only agent that runs on a source host being
// migrated away from. It may only read Docker state and count files; there is
// no code path in it that creates, changes, execs into or deletes anything.
package migrationmode

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ErrDenied wraps every refusal of the Docker allowlist.
var ErrDenied = errors.New("migrationmode: docker call is not on the allowlist")

type rule struct {
	method string
	path   *regexp.Regexp
}

const nameChars = `[A-Za-z0-9][A-Za-z0-9_.-]*`

// Default deny: a call is allowed only if a rule below matches both method and path.
var allowRules = []rule{
	{http.MethodGet, regexp.MustCompile(`^/version$`)},
	{http.MethodGet, regexp.MustCompile(`^/containers/json$`)},
	{http.MethodGet, regexp.MustCompile(`^/containers/` + nameChars + `/json$`)},
	{http.MethodGet, regexp.MustCompile(`^/volumes$`)},
	{http.MethodGet, regexp.MustCompile(`^/volumes/` + nameChars + `$`)},
	{http.MethodGet, regexp.MustCompile(`^/networks$`)},
}

var versionPrefix = regexp.MustCompile(`^/v[0-9]+\.[0-9]+(/|$)`)

// Allowed reports whether method and u may be sent to the Docker socket.
func Allowed(method string, u *url.URL) error {
	if u == nil {
		return fmt.Errorf("%w: no url", ErrDenied)
	}
	if u.RawPath != "" && u.RawPath != u.Path {
		return fmt.Errorf("%w: encoded path", ErrDenied)
	}
	p := u.Path
	if strings.ContainsAny(p, "\x00\\%") || strings.Contains(p, "..") || strings.Contains(p, "//") {
		return fmt.Errorf("%w: suspicious path", ErrDenied)
	}
	if loc := versionPrefix.FindStringIndex(p); loc != nil {
		p = "/" + strings.TrimPrefix(p[loc[1]:], "/")
		if loc[1] == len(u.Path) {
			p = "/"
		}
	}
	for _, r := range allowRules {
		if r.method == method && r.path.MatchString(p) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s %s", ErrDenied, method, p)
}

// DenyFunc is told about every refused call so it can be audited.
type DenyFunc func(method, path, reason string)

type guardTransport struct {
	next   http.RoundTripper
	onDeny DenyFunc
}

// NewGuard wraps next so only allowlisted calls leave the process. A request
// carrying a body is refused as well: every allowed call is a bodiless GET.
func NewGuard(next http.RoundTripper, onDeny DenyFunc) http.RoundTripper {
	return &guardTransport{next: next, onDeny: onDeny}
}

func (g *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	err := Allowed(req.Method, req.URL)
	if err == nil && req.Body != nil && req.Body != http.NoBody {
		err = fmt.Errorf("%w: request body", ErrDenied)
	}
	if err != nil {
		if g.onDeny != nil {
			g.onDeny(req.Method, req.URL.Path, err.Error())
		}
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return g.next.RoundTrip(req)
}
