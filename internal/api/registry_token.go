package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
)

// WithRegistryTokenService enables the registry bearer token endpoint used only
// when build.transfer_mode is registry. Nil keeps it answering 501.
func WithRegistryTokenService(s *build.RegistryTokenService) Option {
	return func(rt *Router) { rt.registryToken = s }
}

// handleRegistryToken is the registry's token realm: the job push token arrives
// as the basic-auth password and leaves as a scoped bearer. Every failure is a bare 401.
func (rt *Router) handleRegistryToken(w http.ResponseWriter, r *http.Request) {
	svc := rt.registryToken
	if svc == nil {
		http.Error(w, "registry token auth is not enabled", http.StatusNotImplemented)
		return
	}
	if ok, retry := rt.codeLogin.redeemByIP.allow("regtok|" + clientIP(r)); !ok {
		writeRateLimited(w, retry)
		return
	}
	deny := func() {
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
	_, pass, ok := r.BasicAuth()
	q := r.URL.Query()
	if !ok || pass == "" || q.Get("service") != svc.Service {
		deny()
		return
	}
	var scopes []string
	for _, v := range q["scope"] {
		scopes = append(scopes, strings.Fields(v)...)
	}
	tok, exp, err := svc.Exchange(pass, scopes)
	if err != nil {
		deny()
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token": tok, "access_token": tok,
		"expires_in": int(time.Until(exp).Seconds()), "issued_at": time.Now().UTC().Format(time.RFC3339),
	})
}
