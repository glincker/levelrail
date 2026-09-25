package models

import (
	"net/http"
	"slices"
	"strings"
)

type routeMatch int

const (
	routeAllowed routeMatch = iota
	routeNotFound
	routeWrongMethod
)

type gatewayRoute struct {
	path   string
	prefix bool
	method string
	// multipart routes carry uploads, so their bodies are size-capped but
	// not parsed as JSON.
	multipart bool
}

var commonRoutes = []gatewayRoute{
	{path: "/v1/models", method: http.MethodGet},
	{path: "/v1/models/", prefix: true, method: http.MethodGet},
	{path: "/v1/chat/completions", method: http.MethodPost},
	{path: "/v1/completions", method: http.MethodPost},
	{path: "/v1/embeddings", method: http.MethodPost},
}

var responsesRoute = gatewayRoute{path: "/v1/responses", method: http.MethodPost}

// engineRoutes lists what each engine serves beyond commonRoutes, from the
// engines' own API docs. Engine control paths are absent on purpose.
var engineRoutes = map[string][]gatewayRoute{
	EngineOllama:   {responsesRoute},
	EngineLlamaCpp: {responsesRoute},
	EngineVLLM: {
		responsesRoute,
		{path: "/v1/audio/transcriptions", method: http.MethodPost, multipart: true},
		{path: "/v1/audio/translations", method: http.MethodPost, multipart: true},
	},
}

func routesFor(engine string) []gatewayRoute {
	return append(slices.Clone(commonRoutes), engineRoutes[engine]...)
}

// AllowedRoutes lists the "METHOD path" pairs the gateway forwards for an
// engine; a trailing slash marks a prefix.
func AllowedRoutes(engine string) []string {
	rs := routesFor(engine)
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.method+" "+r.path)
	}
	return out
}

func (r gatewayRoute) matchesPath(path string) bool {
	if r.prefix {
		return len(path) > len(r.path) && strings.HasPrefix(path, r.path)
	}
	return path == r.path
}

// matchRoute checks path and method against the engine's allowlist. The
// match is exact and case sensitive: the engine sees only what was listed.
func matchRoute(engine, path, method string) (gatewayRoute, routeMatch) {
	if hasDotSegment(path) || strings.ContainsRune(path, '\\') || strings.Contains(path, "//") {
		return gatewayRoute{}, routeNotFound
	}
	pathHit := false
	for _, r := range routesFor(engine) {
		if !r.matchesPath(path) {
			continue
		}
		pathHit = true
		if r.method == method {
			return r, routeAllowed
		}
	}
	if pathHit {
		return gatewayRoute{}, routeWrongMethod
	}
	return gatewayRoute{}, routeNotFound
}

// hasEncodedSlash reports a percent-encoded slash or backslash, which a
// decoded path match cannot see but the engine's router might.
func hasEncodedSlash(rawPath string) bool {
	l := strings.ToLower(rawPath)
	return strings.Contains(l, "%2f") || strings.Contains(l, "%5c")
}

func allowedMethods(engine, path string) string {
	var ms []string
	for _, r := range routesFor(engine) {
		if r.matchesPath(path) && !slices.Contains(ms, r.method) {
			ms = append(ms, r.method)
		}
	}
	return strings.Join(ms, ", ")
}
