package api

import (
	"path"
	"strings"
)

// Route kinds separate pages from the static files and API calls around them.
const (
	routeKindPage  = "page"
	routeKindAPI   = "api"
	routeKindAsset = "asset"
)

var assetPrefixes = []string{
	"/_next/", "/_nuxt/", "/_astro/", "/_app/", "/static/", "/assets/",
	"/build/", "/dist/", "/public/", "/images/", "/img/", "/fonts/",
	"/favicon", "/.well-known/",
}

var assetExtensions = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true, ".png": true,
	".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true,
	".webp": true, ".avif": true, ".woff": true, ".woff2": true, ".ttf": true,
	".otf": true, ".eot": true, ".mp4": true, ".webm": true, ".mp3": true,
	".webmanifest": true, ".txt": true, ".xml": true, ".pdf": true,
}

// classifyRoute is a path-shape heuristic, so it works on stored route
// templates without any change to how requests are recorded.
func classifyRoute(route string) string {
	r := strings.ToLower(route)
	for _, p := range assetPrefixes {
		if strings.HasPrefix(r, p) {
			return routeKindAsset
		}
	}
	if assetExtensions[path.Ext(r)] {
		return routeKindAsset
	}
	if r == "/api" || strings.HasPrefix(r, "/api/") || strings.HasPrefix(r, "/trpc/") || strings.HasPrefix(r, "/graphql") {
		return routeKindAPI
	}
	return routeKindPage
}
