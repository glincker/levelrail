package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
)

const (
	statusPageCacheSeconds = 30
	statusHostCacheTTL     = 30 * time.Second
	statusPageCSP          = "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

// statusHostCache remembers the configured custom domain for a short
// time so every request does not hit the settings table.
type statusHostCache struct {
	mu      sync.Mutex
	domain  string
	expires time.Time
}

func (c *statusHostCache) invalidate() {
	c.mu.Lock()
	c.expires = time.Time{}
	c.mu.Unlock()
}

func (rt *Router) statusCustomDomain(r *http.Request) string {
	c := &rt.statusHost
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.expires) {
		return c.domain
	}
	c.domain = ""
	if s, err := rt.statusPage.GetStatusPageSettings(r.Context()); err == nil && s.Enabled {
		c.domain = s.CustomDomain
	}
	c.expires = time.Now().Add(statusHostCacheTTL)
	return c.domain
}

// StatusHostHandler serves only the public status page when a request's
// Host matches the configured custom domain, and 404s everything else on
// that host so the dashboard and API are never reachable through it.
// Requests for any other host pass through to next untouched.
func (rt *Router) StatusHostHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt.statusPage == nil || rt.statusView == nil {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		domain := rt.statusCustomDomain(r)
		if domain == "" || !strings.EqualFold(host, domain) {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/", statuspage.PagePath:
			rt.servePublicStatus(w, r, "html")
		case "/status.json", statuspage.SummaryPath:
			rt.servePublicStatus(w, r, "json")
		case "/status.rss", statuspage.FeedPath:
			rt.servePublicStatus(w, r, "rss")
		default:
			http.NotFound(w, r)
		}
	})
}

func (rt *Router) handlePublicStatusHTML(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "html")
}

func (rt *Router) handlePublicStatusJSON(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "json")
}

func (rt *Router) handlePublicStatusRSS(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "rss")
}

// servePublicStatus is the single public read path: opt-in, rate
// limited, cacheable, and rendered only from statuspage.View.
func (rt *Router) servePublicStatus(w http.ResponseWriter, r *http.Request, format string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if rt.statusPage == nil || rt.statusView == nil {
		http.NotFound(w, r)
		return
	}
	if rt.statusLimiter != nil {
		if ok, retry := rt.statusLimiter.allow("status|" + clientIP(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
	}
	enabled, err := rt.statusView.Enabled(r.Context())
	if err != nil {
		rt.logger.Error("api: public status: read settings failed", "error", err.Error())
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !enabled {
		http.NotFound(w, r)
		return
	}
	v, err := rt.statusView.View(r.Context())
	if err != nil {
		rt.logger.Error("api: public status: build view failed", "error", err.Error())
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	var (
		body []byte
		ctyp string
	)
	switch format {
	case "json":
		body, err = json.Marshal(v)
		ctyp = "application/json; charset=utf-8"
	case "rss":
		body, err = statuspage.RenderRSS(v, publicBaseURL(r))
		ctyp = "application/rss+xml; charset=utf-8"
	default:
		body, err = statuspage.RenderHTML(v)
		ctyp = "text/html; charset=utf-8"
	}
	if err != nil {
		rt.logger.Error("api: public status: render failed", "error", err.Error())
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	h := w.Header()
	h.Set("Content-Type", ctyp)
	h.Set("Cache-Control", "public, max-age="+strconv.Itoa(statusPageCacheSeconds))
	h.Set("ETag", etag)
	h.Set("Vary", "Host")
	h.Set("Content-Security-Policy", statusPageCSP)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
