package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// registryTokenLimiters keeps one limiter per router, apart from the sign-in buckets.
var registryTokenLimiters sync.Map

func (rt *Router) registryTokenLimiter() *apiRateLimiter {
	if l, ok := registryTokenLimiters.Load(rt); ok {
		return l.(*apiRateLimiter)
	}
	l := newAPIRateLimiter(rt.runtimeSettings.Limit(context.Background(), runtimesettings.KeyRegistryTokenRateLimit))
	l.source = func() int {
		return rt.runtimeSettings.Limit(context.Background(), runtimesettings.KeyRegistryTokenRateLimit)
	}
	actual, _ := registryTokenLimiters.LoadOrStore(rt, l)
	return actual.(*apiRateLimiter)
}

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
	_, pass, ok := r.BasicAuth()
	// Invalid tokens have no nonce and share one bucket per address; a valid job gets its own.
	nonce := ""
	if svc.Issuer != nil {
		nonce = svc.Issuer.Nonce(pass)
	}
	if allowed, retry := rt.registryTokenLimiter().allow(clientIP(r) + "|" + nonce); !allowed {
		writeRateLimited(w, retry)
		return
	}
	deny := func() {
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
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
