package ingress

import (
	"html"
	"strconv"
	"strings"
)

// Maintenance page defaults and bounds.
const (
	DefaultMaintenanceTitle      = "Down for maintenance"
	DefaultMaintenanceMessage    = "This site is temporarily unavailable. Please check back soon."
	DefaultMaintenanceRetryAfter = 300
	MaxMaintenanceTitle          = 120
	MaxMaintenanceMessage        = 2000
)

// MaintenancePage is what visitors see while a domain is in maintenance.
// Empty fields fall back to the defaults.
type MaintenancePage struct {
	Title             string
	Message           string
	RetryAfterSeconds int
}

// MaintenanceGate puts a proxied route behind the maintenance page while
// letting bypass clients and paths through to the full route.
type MaintenanceGate struct {
	Page        MaintenancePage
	BypassCIDRs []string
	BypassPaths []string
}

// ClientIPMatcher mirrors Caddy's http.matchers.client_ip, which honours the
// server's trusted proxies.
type ClientIPMatcher struct {
	Ranges []string `json:"ranges"`
}

func (g *MaintenanceGate) bypassSets() []Matcher {
	var sets []Matcher
	if len(g.BypassCIDRs) > 0 {
		sets = append(sets, Matcher{ClientIP: &ClientIPMatcher{Ranges: g.BypassCIDRs}})
	}
	if len(g.BypassPaths) > 0 {
		sets = append(sets, Matcher{Path: g.BypassPaths})
	}
	return sets
}

// wrap serves the page to everything outside the bypass sets; a gate with
// no bypass serves the page to every request.
func (g *MaintenanceGate) wrap(handle []any) []any {
	page := maintenanceHandle(g.Page)
	sets := g.bypassSets()
	if len(sets) == 0 {
		return page
	}
	return []any{SubrouteHandler{
		Handler: "subroute",
		Routes: []Route{
			{Match: []Matcher{{Not: sets}}, Handle: page},
			{Handle: handle},
		},
	}}
}

// clip trims s and caps it at n characters.
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// maintenancePageHTML is brand neutral, self contained and follows the
// visitor's colour scheme.
const maintenancePageHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>%TITLE%</title>
<style>:root{color-scheme:light dark;--bg:#f6f6f7;--card:#fff;--fg:#18181b;--muted:#52525b;--line:#e4e4e7}@media (prefers-color-scheme:dark){:root{--bg:#0c0c0e;--card:#17171a;--fg:#f4f4f5;--muted:#a1a1aa;--line:#2a2a2e}}*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:16px;background:var(--bg);color:var(--fg);font:16px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif}main{width:100%;max-width:34rem;padding:2rem;background:var(--card);border:1px solid var(--line);border-radius:12px}h1{margin:0 0 .5rem;font-size:1.375rem;line-height:1.3}p{margin:0;color:var(--muted);white-space:pre-line}</style></head>
<body><main><h1>%TITLE%</h1><p>%MESSAGE%</p></main></body>
</html>
`

// NewMaintenancePageHandler builds the 503 maintenance response. Operator
// text is HTML escaped, and Caddy placeholders are escaped so a title can
// never expand into request or environment values.
func NewMaintenancePageHandler(p MaintenancePage) StaticResponseHandler {
	title := clip(p.Title, MaxMaintenanceTitle)
	if title == "" {
		title = DefaultMaintenanceTitle
	}
	msg := clip(p.Message, MaxMaintenanceMessage)
	if msg == "" {
		msg = DefaultMaintenanceMessage
	}
	retry := p.RetryAfterSeconds
	if retry <= 0 {
		retry = DefaultMaintenanceRetryAfter
	}
	body := strings.NewReplacer("%TITLE%", html.EscapeString(title), "%MESSAGE%", html.EscapeString(msg)).Replace(maintenancePageHTML)
	return StaticResponseHandler{
		Handler:    "static_response",
		StatusCode: 503,
		Body:       escapePlaceholders(body),
		Headers: map[string][]string{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Retry-After":   {strconv.Itoa(retry)},
			"Cache-Control": {"no-store"},
		},
	}
}

// NewMaintenanceResponseHandler is the default maintenance page.
func NewMaintenanceResponseHandler() StaticResponseHandler {
	return NewMaintenancePageHandler(MaintenancePage{})
}

// staticAssetPaths are the paths a document pulls in, matched case
// insensitively by Caddy; an HTML body there poisons browser and CDN caches.
var staticAssetPaths = []string{
	"/_next/*", "/static/*", "/assets/*",
	"*.css", "*.js", "*.mjs", "*.map", "*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp",
	"*.avif", "*.svg", "*.ico", "*.woff", "*.woff2", "*.ttf", "*.otf", "*.eot",
	"*.webmanifest", "*.mp4", "*.webm", "*.mp3",
}

// NewAssetUnavailableHandler answers an asset request with an empty, never
// cached 503 so no client ever stores HTML under an asset URL.
func NewAssetUnavailableHandler(retryAfter int) StaticResponseHandler {
	if retryAfter <= 0 {
		retryAfter = DefaultMaintenanceRetryAfter
	}
	return StaticResponseHandler{
		Handler:    "static_response",
		StatusCode: 503,
		Headers: map[string][]string{
			"Content-Type":  {"text/plain; charset=utf-8"},
			"Retry-After":   {strconv.Itoa(retryAfter)},
			"Cache-Control": {"no-store"},
		},
	}
}

// maintenanceHandle serves the page to documents and an empty 503 to assets.
func maintenanceHandle(p MaintenancePage) []any {
	return []any{SubrouteHandler{
		Handler: "subroute",
		Routes: []Route{
			{Match: []Matcher{{Path: staticAssetPaths}}, Handle: []any{NewAssetUnavailableHandler(p.RetryAfterSeconds)}},
			{Handle: []any{NewMaintenancePageHandler(p)}},
		},
	}}
}
