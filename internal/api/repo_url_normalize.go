package api

import (
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider/reporef"
)

// normalizePastedRepoURL turns what an operator pastes into a plain https
// repository URL plus a branch and directory when the URL pins them.
func normalizePastedRepoURL(raw string) (repoURL, ref, dir string) {
	s := strings.TrimSpace(raw)
	if r, err := reporef.Parse(s); err == nil && !r.IsHTTP() {
		s = r.CloneURL()
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return raw, "", ""
	}
	u.RawQuery, u.Fragment = "", ""
	p := strings.Trim(u.Path, "/")
	if i := strings.Index(p, "/tree/"); i > 0 {
		rest := strings.SplitN(p[i+len("/tree/"):], "/", 2)
		ref = rest[0]
		if len(rest) == 2 {
			dir = rest[1]
		}
		p = p[:i]
	}
	u.Path = strings.TrimSuffix(p, ".git")
	if u.Path != "" {
		u.Path = "/" + u.Path
	}
	return u.String(), ref, dir
}
