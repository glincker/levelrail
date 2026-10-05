package api

import (
	"net/http"
	"net/url"
	"strings"
)

// csrfOriginMiddleware rejects cookie-authenticated state-changing requests
// whose Origin names a different host than the one served. SameSite=Lax is
// the first line; this closes same-site sibling-subdomain and legacy-browser gaps.
func csrfOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if csrfCrossOrigin(r) {
			writeError(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfCrossOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if r.Header.Get("Authorization") != "" {
		return false
	}
	if _, err := r.Cookie(sessionCookieName); err != nil {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return origin == "null"
	}
	u, err := url.Parse(origin)
	if err != nil {
		return true
	}
	return !strings.EqualFold(u.Host, r.Host)
}
