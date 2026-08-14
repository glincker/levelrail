// Package web embeds the built frontend via embed.FS, keeping the
// control plane a single static binary with no Node runtime required on
// the server, and serves it with client-side-routing fallback, so a hard
// refresh or direct link to a TanStack Router path like /apps/foo resolves
// correctly instead of 404ing at the server before React Router ever
// gets a chance to handle it.
package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// Handler serves the embedded frontend. If this binary wasn't built
// with -tags embedweb (the default, see embed_stub.go), it serves a 501
// with an actionable message instead of panicking or 404ing silently:
// a missing frontend is a build configuration fact worth surfacing
// clearly, not a routing failure to debug.
func Handler() http.Handler {
	return handlerFromFS(DistFS)
}

func handlerFromFS(embedded fs.FS) http.Handler {
	// fs.Sub doesn't fail just because "dist" is missing or empty: for
	// the generic (non-SubFS) case it builds a lazy view and only
	// errors on the first actual file access, which would otherwise
	// surface as a bare, unexplained 404 on every request rather than
	// the actionable message below. Probing for index.html once here,
	// at handler construction time, tells the two cases apart up front.
	if dist, err := fs.Sub(embedded, "dist"); err == nil {
		if _, err := fs.Stat(dist, "index.html"); err == nil {
			return spaHandler{dist: dist, fileServer: http.FileServer(http.FS(dist))}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w,
			"frontend not embedded in this binary: build with `cd web && npm run build && go build -tags embedweb ./...`",
			http.StatusNotImplemented)
	})
}

type spaHandler struct {
	dist       fs.FS
	fileServer http.Handler
}

// ServeHTTP serves a real static asset when the request path matches
// one, and falls back to dist/index.html otherwise: the standard
// single-page-app pattern, needed because every client-side route
// (/apps, /apps/foo, /apps/foo/env, ...) has no matching file on disk,
// only the app shell that renders them.
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if _, err := fs.Stat(h.dist, name); err != nil {
		http.ServeFileFS(w, r, h.dist, "index.html")
		return
	}
	h.fileServer.ServeHTTP(w, r)
}
