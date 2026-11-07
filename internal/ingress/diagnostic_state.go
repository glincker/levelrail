package ingress

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/net/idna"
)

// Runtime setting keys and env seeds: how often the host state snapshot
// refreshes, and whether the page may reveal a host's state at all.
const (
	KeyDiagnosticCacheTTLSeconds = "ingress.diagnostic_cache_ttl_seconds"
	EnvDiagnosticCacheTTLSeconds = "LEVELRAIL_INGRESS_DIAGNOSTIC_CACHE_TTL_SECONDS"
	KeyDiagnosticShowState       = "ingress.diagnostic_show_state"
	EnvDiagnosticShowState       = "LEVELRAIL_INGRESS_DIAGNOSTIC_SHOW_STATE"
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
	// StateNotLoaded means no snapshot has been built yet, so nothing is known.
	StateNotLoaded HostState = "not_loaded"
)

// HostInfo is what a lookup returns. It never carries an app name: the
// public page must not disclose internal names.
type HostInfo struct {
	State HostState
}

// NormalizeHost lowercases, strips a port and trailing dot, and converts to
// ASCII (punycode) so both sides of a match compare equal.
func NormalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if strings.HasPrefix(h, "[") {
		if i := strings.Index(h, "]"); i > 0 {
			h = h[1:i]
		}
	} else if i := strings.LastIndex(h, ":"); i >= 0 && strings.Count(h, ":") == 1 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		h = a
	}
	return h
}

// HostLookup maps a lowercase host to its state; it must not hit the DB per call.
type HostLookup func(ctx context.Context, host string) HostInfo

type stateContentSet struct{ headline, body, next string }

func cmdItem(label, cmd string) string {
	return `<li>` + label + `<code class="cmd">` + cmd + `</code></li>`
}

func stateContent(info HostInfo) (stateContentSet, bool) {
	const app = "&lt;app&gt;"
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

// DiagnosticHandler answers a request for a host with no answering app: HTML
// for browsers, short plain text for other clients. Unless showState is true
// every host gets the same generic 404, so probing learns nothing.
func DiagnosticHandler(lookup HostLookup, showState func(context.Context) bool, brandName, docsURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := HostInfo{State: StateNotLoaded}
		if lookup != nil && showState != nil && showState(r.Context()) {
			info = lookup(r.Context(), NormalizeHost(r.Host))
		}
		if info.State == StateNotLoaded {
			WriteGenericDiagnostic(w, r, brandName, docsURL)
			return
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

// WriteGenericDiagnostic is the same 404 body the static ingress page gives.
func WriteGenericDiagnostic(w http.ResponseWriter, r *http.Request, brandName, docsURL string) {
	w.Header().Set("Cache-Control", "no-store")
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(renderDiagnosticHTML(http.StatusNotFound, HostInfo{}, brandName, docsURL)))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(diagnosticMessage404 + "\n"))
}
