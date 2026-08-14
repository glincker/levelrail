package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// staticSiteResource is a static site's wire shape: name and domains
// only, deliberately omitting store.StaticSite's RootDir. RootDir is an
// absolute filesystem path on the control plane host (see
// internal/store/static_site.go's own doc comment: "the local
// filesystem directory internal/deploy copied this site's built static
// files into"), used exclusively by internal/reconcile/ingress to hand
// Caddy's file_server handler a root. No frontend surface needs a local
// server path (there's nothing to link to or act on with it, unlike
// Domain which is directly user-facing), and shipping a control plane's
// own directory layout to every authenticated client is a small,
// avoidable disclosure with no offsetting benefit. If a future UI needs
// to prove where a site's files live (a debug panel, say), add it back
// deliberately then, not speculatively now.
type staticSiteResource struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

// StaticSiteStore is the store surface handleListStaticSites needs.
// *store.DB satisfies this structurally, the same consumer-defined
// interface convention every other Store sub-interface in this package
// follows (see CertStore, DatabaseStore).
type StaticSiteStore interface {
	ListStaticSites(ctx context.Context) ([]store.StaticSite, error)
}

// handleListStaticSites handles GET /api/v1/static-sites: every
// build.type: static site (internal/store/static_site.go)
// this control plane currently serves directly through embedded Caddy,
// with no container involved. Before this route existed, a static site
// deployed via git push had no API surface and no dashboard visibility
// at all, the exact follow-up gap the builder who shipped static-site
// support (migration 0015) flagged and didn't action; this closes it the
// same "read status data and shape it into a response" way
// handleListDatabaseEngines (database_engines.go) does for its own
// simple, path-param-free store list. A control plane with no static
// sites deployed yet returns an empty list, not an error, the same
// "absence is a real, reportable state, never a 5xx" shape
// handleListCertificates already establishes.
func (rt *Router) handleListStaticSites(w http.ResponseWriter, r *http.Request) {
	sites, err := rt.staticSites.ListStaticSites(r.Context())
	if err != nil {
		rt.logger.Error("api: list static sites failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]staticSiteResource, 0, len(sites))
	for _, s := range sites {
		out = append(out, staticSiteResource{
			Name:    s.Name,
			Domains: s.Domains,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
