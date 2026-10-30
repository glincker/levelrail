package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
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
	statusAssetCSP         = "default-src 'none'; base-uri 'none'; frame-ancestors 'none'"
)

// statusHTMLCSP admits images from the configured logo origin only.
func statusHTMLCSP(logoURL string) string {
	u, err := url.Parse(logoURL)
	if logoURL == "" || err != nil || u.Scheme != "https" || u.Host == "" {
		return statusPageCSP
	}
	return statusPageCSP + "; img-src https://" + u.Host
}

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
	// Keep the domain while the page is disabled or settings are unreadable so
	// the host stays locked to public routes (servePublicStatus 404s when off).
	if s, err := rt.statusPage.GetStatusPageSettings(r.Context()); err == nil {
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
		path := r.URL.Path
		switch path {
		case "/", statuspage.PagePath:
			rt.servePublicStatus(w, r, "html")
		case "/status.json", statuspage.SummaryPath:
			rt.servePublicStatus(w, r, "json")
		case "/status.rss", statuspage.FeedPath:
			rt.servePublicStatus(w, r, "rss")
		case "/status.atom", statuspage.AtomPath:
			rt.servePublicStatus(w, r, "atom")
		case "/status/history", statuspage.HistoryPath:
			rt.servePublicStatus(w, r, "history")
		case "/status/badge.svg", statuspage.BadgePath:
			rt.servePublicStatus(w, r, "badge")
		default:
			id, ok := strings.CutPrefix(path, "/status/incidents/")
			if !ok {
				id, ok = strings.CutPrefix(path, statuspage.IncidentDir)
			}
			if !ok || id == "" || strings.Contains(id, "/") {
				http.NotFound(w, r)
				return
			}
			r.SetPathValue("id", id)
			rt.servePublicStatus(w, r, "incident")
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

func (rt *Router) handlePublicStatusAtom(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "atom")
}

func (rt *Router) handlePublicStatusHistory(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "history")
}

func (rt *Router) handlePublicStatusIncident(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "incident")
}

func (rt *Router) handlePublicStatusBadge(w http.ResponseWriter, r *http.Request) {
	rt.servePublicStatus(w, r, "badge")
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
	var v statuspage.View
	if format == "history" || format == "incident" {
		v, err = rt.statusView.History(r.Context())
	} else {
		v, err = rt.statusView.View(r.Context())
	}
	if err != nil {
		rt.logger.Error("api: public status: build view failed", "error", err.Error())
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	var (
		body []byte
		ctyp string
		csp  = statusHTMLCSP(v.LogoURL)
	)
	switch format {
	case "json":
		body, err = json.Marshal(v)
		ctyp = "application/json; charset=utf-8"
	case "rss":
		body, err = statuspage.RenderRSS(v, publicBaseURL(r))
		ctyp = "application/rss+xml; charset=utf-8"
	case "atom":
		body, err = statuspage.RenderAtom(v, publicBaseURL(r))
		ctyp, csp = "application/atom+xml; charset=utf-8", statusAssetCSP
	case "badge":
		body = renderStatusBadge(v, r.URL.Query().Get("component"))
		if body == nil {
			http.NotFound(w, r)
			return
		}
		ctyp, csp = "image/svg+xml", statusAssetCSP
	case "history":
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		body, err = statuspage.RenderHistory(v, r.URL.Query().Get("tab"), page, statuspage.TuningFromEnv().HistoryPage)
		ctyp = "text/html; charset=utf-8"
	case "incident":
		var found bool
		body, found, err = statuspage.RenderIncident(v, r.PathValue("id"))
		if err == nil && !found {
			http.NotFound(w, r)
			return
		}
		ctyp = "text/html; charset=utf-8"
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
	h.Set("Content-Security-Policy", csp)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body) //nolint:gosec // G705: body is rendered by internal/statuspage, which escapes every operator-supplied value
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// renderStatusBadge returns the overall badge, or the badge of the
// component with the given ID; nil means the component does not exist.
func renderStatusBadge(v statuspage.View, componentID string) []byte {
	if componentID == "" {
		return statuspage.RenderBadge("status", v.Status)
	}
	for _, c := range v.Components {
		if c.ID == componentID {
			return statuspage.RenderBadge(c.Name, c.Status)
		}
	}
	return nil
}
