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
<style>:root{color-scheme:light dark;--bg:#f6f6f7;--card:#fff;--fg:#18181b;--muted:#52525b;--line:#e4e4e7}@media (prefers-color-scheme:dark){:root{--bg:#0c0c0e;--card:#17171a;--fg:#f4f4f5;--muted:#a1a1aa;--line:#2a2a2e}}*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:16px;background:var(--bg);color:var(--fg);font:16px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif}main{width:100%;max-width:36rem;padding:2rem;background:var(--card);border:1px solid var(--line);border-radius:12px}h1{margin:0 0 .5rem;font-size:1.375rem;line-height:1.3}p,li{color:var(--muted)}code{color:var(--fg);word-break:break-all}a{color:inherit}small{color:var(--muted)}</style></head>
<body><main><h1>No app is serving this address</h1>
<p>Requested host: <code id="h"></code> (HTTP %STATUS%)</p>
<ul><li>The app is stopped or still deploying.</li><li>The app has no healthy replica.</li><li>This hostname is not attached to an app.</li></ul>
<p><a href="%DOCS%">How domains work</a></p><small>%BRAND%</small></main>
<script>document.getElementById("h").textContent=location.hostname</script></body>
</html>
`

// diagnosticRoutes answers an unmatched or backendless request: HTML for
// browsers, short plain text for everything else. The host is filled client
// side so it is never interpolated into markup.
func diagnosticRoutes(status int, brandName, docsURL string) []Route {
	msg := diagnosticMessage503
	if status == 404 {
		msg = diagnosticMessage404
	}
	page := strings.NewReplacer(
		"%STATUS%", strconv.Itoa(status),
		"%DOCS%", html.EscapeString(docsURL),
		"%BRAND%", html.EscapeString(brandName),
	).Replace(diagnosticPageHTML)
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

// diagnosticCatchAll is the last route: any host no route claimed gets the 404 page.
func diagnosticCatchAll(h *Hardening) Route {
	return Route{Handle: []any{SubrouteHandler{Handler: "subroute", Routes: diagnosticRoutes(404, h.BrandName, h.DocsURL)}}}
}

// diagnostic503Route covers the error path, where Caddy reports "no upstreams available".
func diagnostic503Route(h Hardening) Route {
	return Route{
		Match:  []Matcher{{Expression: &ExpressionMatcher{Expr: "{http.error.status_code} == 503"}}},
		Handle: []any{SubrouteHandler{Handler: "subroute", Routes: diagnosticRoutes(503, h.BrandName, h.DocsURL)}},
	}
}
