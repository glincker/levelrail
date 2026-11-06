package ingress

import (
	"context"
	"html"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Runtime setting key and env seed for how long a host's state is reused.
const (
	KeyDiagnosticCacheTTLSeconds = "ingress.diagnostic_cache_ttl_seconds"
	EnvDiagnosticCacheTTLSeconds = "LEVELRAIL_INGRESS_DIAGNOSTIC_CACHE_TTL_SECONDS"
	diagnosticCacheMaxHosts      = 2048
)

// HostState is the real reason a host has no answering app.
type HostState string

// The possible host states, from most to least specific.
const (
	StateUnattached HostState = "unattached"
	StateBuilding   HostState = "building"
	StateStopped    HostState = "stopped"
	StateStarting   HostState = "starting"
	StateUnhealthy  HostState = "unhealthy"
	StateUnknown    HostState = "unknown"
)

// HostInfo is what a lookup returns. App is set only when the host is
// attached to it, so an unattached host never leaks an app name.
type HostInfo struct {
	State HostState
	App   string
}

// HostLookup maps a lowercase host to its state; it must not hit the DB per call.
type HostLookup func(ctx context.Context, host string) HostInfo

type stateContentSet struct{ headline, body, next string }

func cmdItem(label, cmd string) string {
	return `<li>` + label + `<code class="cmd">` + cmd + `</code></li>`
}

func stateContent(info HostInfo) (stateContentSet, bool) {
	app := "&lt;app&gt;"
	if info.App != "" {
		app = html.EscapeString(info.App)
	}
	switch info.State {
	case StateUnattached:
		return stateContentSet{"This address is not attached to an app",
			`<p>No app has this hostname, or the app behind it was removed.</p>`,
			`<ol>` + cmdItem("Attach it to an app:", "levelrail-cli domains add &lt;app&gt; &lt;host&gt;") + cmdItem("Or list what is attached:", "levelrail-cli domains list") + `</ol>`}, true
	case StateBuilding:
		return stateContentSet{"Your app is still building",
			`<p>A deploy is building or queued. This address starts answering when it finishes.</p>`,
			`<ol>` + cmdItem("Watch the deploy:", "levelrail-cli apps deploys list "+app) + `</ol>`}, true
	case StateStopped:
		return stateContentSet{"The app is stopped",
			`<p>The app is stopped or scaled to zero replicas.</p>`,
			`<ol>` + cmdItem("Start it:", "levelrail-cli apps start "+app) + `</ol>`}, true
	case StateStarting:
		return stateContentSet{"The app is starting",
			`<p>Replicas are starting. This page clears once the first one is ready.</p>`,
			`<ol>` + cmdItem("Follow startup:", "levelrail-cli logs "+app) + `</ol>`}, true
	case StateUnhealthy:
		return stateContentSet{"The app has no healthy replica",
			`<p>The app is running but no replica passes its health check.</p>`,
			`<ol>` + cmdItem("Read the logs for crashes:", "levelrail-cli logs "+app) + cmdItem("Check the latest deploy:", "levelrail-cli apps deploys list "+app) + `</ol>`}, true
	}
	return stateContentSet{}, false
}

// CachedLookup reuses results for a TTL read live, so a settings change applies
// without a restart. The map is capped so random hostnames cannot grow it.
func CachedLookup(inner HostLookup, ttl func(context.Context) time.Duration) HostLookup {
	type entry struct {
		info HostInfo
		at   time.Time
	}
	var mu sync.Mutex
	cache := map[string]entry{}
	return func(ctx context.Context, host string) HostInfo {
		d := ttl(ctx)
		now := time.Now()
		mu.Lock()
		e, ok := cache[host]
		mu.Unlock()
		if ok && d > 0 && now.Sub(e.at) < d {
			return e.info
		}
		info := inner(ctx, host)
		if d > 0 {
			mu.Lock()
			if len(cache) >= diagnosticCacheMaxHosts {
				cache = map[string]entry{}
			}
			cache[host] = entry{info, now}
			mu.Unlock()
		}
		return info
	}
}

// DiagnosticHandler answers a request for a host with no answering app: HTML
// for browsers, short plain text for other clients. Never reveals env or IPs.
func DiagnosticHandler(lookup HostLookup, brandName, docsURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		info := HostInfo{State: StateUnknown}
		if lookup != nil {
			info = lookup(r.Context(), host)
		}
		status := http.StatusServiceUnavailable
		if info.State == StateUnattached {
			status = http.StatusNotFound
		}
		w.Header().Set("Cache-Control", "no-store")
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(renderDiagnosticHTML(status, info, brandName, docsURL)))
			return
		}
		msg := diagnosticMessage503
		if c, ok := stateContent(info); ok {
			msg = c.headline + "."
		} else if status == http.StatusNotFound {
			msg = diagnosticMessage404
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(msg + "\n"))
	})
}
