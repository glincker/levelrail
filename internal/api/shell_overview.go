package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// defaultShellSectionTimeout applies only when no runtime settings service is wired.
const defaultShellSectionTimeout = 2 * time.Second

// shellSections are the GETs the dashboard shell fires before first paint.
// The key is the section name the web client maps to its query cache key.
var shellSections = []struct {
	name string
	path string
}{
	{"brand", "/api/v1/brand"},
	{"demo", "/api/v1/demo"},
	{"dev_mode", "/api/v1/dev-mode"},
	{"session", "/api/v1/auth/session"},
	{"preferences", "/api/v1/auth/preferences"},
	{"appearance", "/api/v1/me/preferences/appearance"},
	{"local_mode", "/api/v1/local-mode"},
	{"system_status", "/api/v1/system/status"},
	{"sign_in_requests", "/api/v1/auth/sign-in-requests"},
	{"projects", "/api/v1/projects"},
	{"teams", "/api/v1/teams"},
}

// shellOverview replays each section through the full router chain, so every
// section gets the exact authz, team gate and scoping of its own endpoint.
type shellOverview struct {
	inner   http.Handler
	timeout func(*http.Request) time.Duration
}

type shellOverviewResponse struct {
	Sections map[string]json.RawMessage `json:"sections"`
}

// handleShellOverview serves GET /api/v1/shell. A section the caller may not
// read (or that fails) is left out; the individual endpoints stay for refetch.
func (s *shellOverview) handleShellOverview(w http.ResponseWriter, r *http.Request) {
	out := shellOverviewResponse{Sections: make(map[string]json.RawMessage, len(shellSections))}
	if s.inner != nil {
		limit := defaultShellSectionTimeout
		if s.timeout != nil {
			limit = s.timeout(r)
		}
		// Sections run side by side; each result lands in its own slot, so output is deterministic.
		bodies := make([]json.RawMessage, len(shellSections))
		var wg sync.WaitGroup
		for i, sec := range shellSections {
			wg.Go(func() { bodies[i], _ = s.fetchSection(r, sec.path, limit) })
		}
		wg.Wait()
		for i, sec := range shellSections {
			if bodies[i] != nil {
				out.Sections[sec.name] = bodies[i]
			}
		}
	}
	// Per user body: no-store keeps it out of every cache and skips the shared ETag.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Cookie, Authorization")
	writeJSON(w, http.StatusOK, out)
}

// fetchSection builds a fresh request with only the caller's credentials and
// none of the outer request's context values, so no resolved scope leaks in.
func (s *shellOverview) fetchSection(r *http.Request, path string, limit time.Duration) (json.RawMessage, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	sub, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false
	}
	sub.Host, sub.RemoteAddr, sub.TLS = r.Host, r.RemoteAddr, r.TLS
	for k, v := range r.Header {
		switch http.CanonicalHeaderKey(k) {
		case "If-None-Match", "If-Modified-Since", "Accept-Encoding", "Range":
			continue
		}
		sub.Header[k] = v
	}
	rec := &shellSectionWriter{header: http.Header{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.inner.ServeHTTP(rec, sub)
	}()
	// A handler that ignores its context is abandoned at the deadline; only it touches rec.
	select {
	case <-done:
	case <-ctx.Done():
		return nil, false
	}
	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	if status != http.StatusOK || !strings.HasPrefix(rec.header.Get("Content-Type"), "application/json") {
		return nil, false
	}
	body := bytes.TrimSpace(rec.body.Bytes())
	if !json.Valid(body) {
		return nil, false
	}
	return json.RawMessage(body), true
}

type shellSectionWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *shellSectionWriter) Header() http.Header { return w.header }

func (w *shellSectionWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *shellSectionWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

// shellSectionTimeout reads the per section deadline live.
func (rt *Router) shellSectionTimeout(r *http.Request) time.Duration {
	if rt.runtimeSettings != nil {
		if d := rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyShellSectionTimeout); d > 0 {
			return d
		}
	}
	return defaultShellSectionTimeout
}
