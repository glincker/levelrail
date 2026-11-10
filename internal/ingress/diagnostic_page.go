package ingress

import (
	"html"
	"strconv"
	"strings"
)

const (
	diagnosticMessage503 = "No app is serving this address. The app may be stopped or still deploying, it may have no healthy replica, or the hostname may not be attached to an app."
	diagnosticMessage404 = "No app is serving this address. The hostname is not attached to an app, or the app behind it was removed."
)

const diagnosticPageHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>%STATUS% No app is serving this address</title>
<style>:root{color-scheme:light dark;--bg:#f6f6f7;--card:#fff;--fg:#18181b;--muted:#52525b;--line:#e4e4e7;--code:#f0f0f2;--accent:#0b6482}@media (prefers-color-scheme:dark){:root{--bg:#0c0c0e;--card:#17171a;--fg:#f4f4f5;--muted:#a1a1aa;--line:#2a2a2e;--code:#232327;--accent:#58b1ce}}*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:16px;background:var(--bg);color:var(--fg);font:16px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif}main{width:100%;max-width:36rem;padding:1.5rem;background:var(--card);border:1px solid var(--line);border-radius:12px}header{display:flex;align-items:center;gap:.625rem;margin-bottom:1rem}header svg{width:28px;height:28px;flex:none}header span{font-weight:600;font-size:.9375rem}h1{margin:0 0 .5rem;font-size:1.375rem;line-height:1.3}h2{margin:1.25rem 0 .5rem;font-size:.9375rem}p,li{color:var(--muted);margin:0 0 .5rem}ol{margin:0;padding-left:1.25rem}code{color:var(--fg);background:var(--code);border-radius:4px;padding:.1em .35em;font-size:.875em;overflow-wrap:anywhere}.cmd{display:block;margin:.25rem 0;padding:.4rem .6rem;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}a{color:var(--accent)}a:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:2px}footer{margin-top:1.25rem;padding-top:.75rem;border-top:1px solid var(--line);font-size:.875rem;color:var(--muted)}</style></head>
<body><main>
<header><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" role="img" aria-label="Levelrail"><rect x="10" y="174" width="236" height="68" rx="22" fill="#06232C"/><rect x="10" y="162" width="236" height="68" rx="22" fill="#084F67"/><rect x="42" y="103" width="172" height="68" rx="22" fill="#06232C"/><rect x="42" y="91" width="172" height="68" rx="22" fill="#107292"/><rect x="75" y="32" width="106" height="68" rx="22" fill="#06232C"/><rect x="75" y="20" width="106" height="68" rx="22" fill="#58B1CE"/></svg><span>%BRAND%</span></header>
<h1>%HEADLINE%</h1>
<p>Requested host: <code id="h"></code> (HTTP %STATUS%)</p>
%CASE%
<h2>What to do next</h2>
%NEXT%
<footer><a href="%DOCS%">How domains work</a></footer></main>
<script>document.getElementById("h").textContent=location.hostname</script></body>
</html>
`

// Ingress only knows the status code: 404 means no route claimed the host,
// 503 means a route exists but has no healthy upstream. No DB lookups here.
const diagnosticGenericNext = `<ol><li>List domains and check the hostname is attached:<code class="cmd">levelrail-cli domains list</code></li><li>Check the latest deploy finished:<code class="cmd">levelrail-cli apps deploys list &lt;app&gt;</code></li><li>Read the app logs for crashes:<code class="cmd">levelrail-cli logs &lt;app&gt;</code></li></ol>`

const (
	diagnosticCase404 = `<p>This hostname is not attached to any app, or the app behind it was removed.</p>`
	diagnosticCase503 = `<p>This hostname is attached, but no healthy replica is answering. Most likely causes, in order:</p><ol><li>A deploy is still building or starting.</li><li>The app is stopped.</li><li>The app is crashing or failing its health check.</li></ol>`
)

// diagnosticRoutes answers an unmatched or backendless request: HTML for
// browsers, short plain text for everything else. The host is filled client
// side so it is never interpolated into markup.
func diagnosticRoutes(status int, brandName, docsURL string) []Route {
	msg := diagnosticMessage503
	if status == 404 {
		msg = diagnosticMessage404
	}
	page := renderDiagnosticHTML(status, HostInfo{}, brandName, docsURL)
	headers := func(ct string) map[string][]string {
		return map[string][]string{"Content-Type": {ct}, "Cache-Control": {"no-store"}}
	}
	return []Route{
		{
			Match: []Matcher{{Header: map[string][]string{"Accept": {"*text/html*"}}}},
			Handle: []any{StaticResponseHandler{
				Handler: "static_response", StatusCode: status,
				Body: escapePlaceholders(page), Headers: headers("text/html; charset=utf-8"),
			}},
		},
		{
			Handle: []any{StaticResponseHandler{
				Handler: "static_response", StatusCode: status,
				Body:    escapePlaceholders(msg) + " Host: {http.request.host}\n",
				Headers: headers("text/plain; charset=utf-8"),
			}},
		},
	}
}

// renderDiagnosticHTML fills the page for one status and, when known, one
// real cause. With no state it keeps the generic static wording.
func renderDiagnosticHTML(status int, info HostInfo, brandName, docsURL string) string {
	headline, body, next := "No app is serving this address", diagnosticCaseHTML(status), diagnosticGenericNext
	if c, ok := stateContent(info); ok {
		headline, body, next = c.headline, c.body, c.next
	}
	return strings.NewReplacer(
		"%HEADLINE%", html.EscapeString(headline),
		"%CASE%", body,
		"%NEXT%", next,
		"%STATUS%", strconv.Itoa(status),
		"%DOCS%", html.EscapeString(docsURL),
		"%BRAND%", html.EscapeString(brandName),
	).Replace(diagnosticPageHTML)
}

func diagnosticCaseHTML(status int) string {
	if status == 404 {
		return diagnosticCase404
	}
	return diagnosticCase503
}

// diagnosticCatchAll is the last route: any host no route claimed gets the 404 page.
func diagnosticCatchAll(h *Hardening) Route {
	if r, ok := unmatchedCatchAll(h); ok {
		return r
	}
	if h.DiagnosticDial != "" && h.DiagnosticToken != "" {
		return diagnosticProxyRoute(h)
	}
	return Route{Handle: []any{SubrouteHandler{Handler: "subroute", Routes: diagnosticRoutes(404, h.BrandName, h.DocsURL)}}}
}

// diagnostic503Route covers the error path, where Caddy reports "no upstreams available".
func diagnostic503Route(h Hardening) Route {
	return Route{
		Match:  []Matcher{{Expression: &ExpressionMatcher{Expr: "{http.error.status_code} == 503"}}},
		Handle: []any{SubrouteHandler{Handler: "subroute", Routes: diagnosticRoutes(503, h.BrandName, h.DocsURL)}},
	}
}

// plainHTTPDiagnostic is the port 80 fallback: it needs no certificate, and
// the control plane hop (when wired) applies the show-state setting.
func plainHTTPDiagnostic(h *Hardening) Route {
	if h == nil {
		d := DefaultHardening()
		h = &d
	}
	return diagnosticCatchAll(h)
}
