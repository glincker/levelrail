package preview

import (
	"net/url"
	"strconv"
	"strings"
)

var authRedirectWords = []string{"login", "signin", "sign-in", "sign_in", "auth", "sso", "oauth"}

// Classify says whether a browser result allows a thumbnail. A non-empty
// reason means skip. requestedPath is the path that was asked for: landing
// on a different login-looking path means an auth wall.
func (r *ShotResult) Classify(requestedPath string) (reason, detail string) {
	switch {
	case r.HTTPStatus == 0:
		return ReasonUnreachable, "the app did not answer on its network"
	case r.HTTPStatus == 401 || r.HTTPStatus == 403:
		return ReasonAuthWall, "the app answered " + strconv.Itoa(r.HTTPStatus)
	case r.HTTPStatus < 200 || r.HTTPStatus >= 300:
		return ReasonHTTPStatus, "the app answered " + strconv.Itoa(r.HTTPStatus)
	}
	if u, err := url.Parse(r.FinalURL); err == nil {
		final := strings.ToLower(u.Path)
		if final != strings.ToLower(requestedPath) {
			for _, w := range authRedirectWords {
				if strings.Contains(final, w) && !strings.Contains(strings.ToLower(requestedPath), w) {
					return ReasonAuthWall, "the app redirected to a login page"
				}
			}
		}
	}
	return "", ""
}
