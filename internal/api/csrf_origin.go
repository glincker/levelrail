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
	if origin := r.Header.Get("Origin"); origin != "" {
		return !sameHost(origin, r.Host)
	}
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "cross-site", "same-site":
		return true
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return !sameHost(ref, r.Host)
	}
	return false
}

func sameHost(raw, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
