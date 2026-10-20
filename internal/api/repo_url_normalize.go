package api

import (
	"net/url"
	"regexp"
	"strings"
)

var scpLikeRepoURL = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?([A-Za-z0-9.-]+\.[A-Za-z]{2,}):([^/\s][^\s]*)$`)

// normalizePastedRepoURL turns what an operator pastes into a plain https
// repository URL plus a branch and directory when the URL pins them.
func normalizePastedRepoURL(raw string) (repoURL, ref, dir string) {
	s := strings.TrimSpace(raw)
	if m := scpLikeRepoURL.FindStringSubmatch(s); m != nil && !strings.Contains(s, "://") {
		s = "https://" + m[1] + "/" + m[2]
	}
	if strings.HasPrefix(s, "ssh://") || strings.HasPrefix(s, "git://") {
		s = "https://" + strings.TrimPrefix(strings.TrimPrefix(s, "ssh://"), "git://")
		if u, err := url.Parse(s); err == nil {
			u.User = nil
			s = u.String()
		}
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
