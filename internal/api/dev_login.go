package api

import (
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// devLoginMinTokenLen is the shortest APP_DEV_LOGIN_TOKEN accepted.
const devLoginMinTokenLen = 32

const accountEventDevTokenLogin = "account.dev_token_login" //nolint:gosec // audit action name, not a credential

// devLoginAllowedHost reports whether the configured public host is a local
// development host; anything else means a real install and refuses the route.
func devLoginAllowedHost(publicHost string) bool {
	switch strings.ToLower(strings.TrimSpace(publicHost)) {
	case "localhost", "127.0.0.1":
		return true
	}
	return false
}

// isLoopbackHostHeader accepts loopback addresses and *.localhost names only.
func isLoopbackHostHeader(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// safeDevLoginNext returns next only when it is a same-origin relative path.
func safeDevLoginNext(next string) string {
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return next
}

// devLoginEnabled decides at boot whether the route may answer at all, and
// logs one warning when it is configured but refused or active.
func (rt *Router) devLoginEnabled() (string, bool) {
	token, publicHost := devLoginConfig()
	if token == "" {
		return "", false
	}
	switch {
	case !devModeEnabled():
		rt.logger.Warn("api: APP_DEV_LOGIN_TOKEN ignored: dev mode is not enabled in this build")
	case len(token) < devLoginMinTokenLen:
		rt.logger.Warn("api: APP_DEV_LOGIN_TOKEN ignored: token is too short")
	case !devLoginAllowedHost(publicHost):
		rt.logger.Warn("api: APP_DEV_LOGIN_TOKEN ignored: public host is not local")
	default:
		rt.logger.Warn("api: dev token login is enabled for local development, never use on a real install")
		return token, true
	}
	return "", false
}

// handleDevLogin handles GET /api/v1/auth/dev-login: local dev and test only.
// The token is accepted in the query string alone and never logged.
func (rt *Router) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	token, ok := rt.devLoginToken, rt.devLoginToken != ""
	if !ok || !isLoopbackRemote(r.RemoteAddr) || !isLoopbackHostHeader(r.Host) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if !rt.allowTokenRedeem(w, r, "dev-login") {
		return
	}
	got := r.URL.Query().Get("token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	user, err := rt.auth.GetUserByEmail(r.Context(), devModeUsername)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := rt.establishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: dev login: create session failed", err)
		return
	}
	rt.recordAccountEvent(r.Context(), r, user.ID, accountEventDevTokenLogin, http.StatusFound)
	rt.logger.Info("api: dev token login", slog.String("user_id", user.ID))
	// next is validated to a same-origin relative path by safeDevLoginNext.
	http.Redirect(w, r, safeDevLoginNext(r.URL.Query().Get("next")), http.StatusFound) //nolint:gosec // see safeDevLoginNext
}
