package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/pagespreviews"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// staticPreviewEntry is the latest preview deploy of one branch of a Pages site.
type staticPreviewEntry struct {
	Branch    string `json:"branch"`
	PRNumber  int    `json:"pr_number"`
	Site      string `json:"site"`
	CommitSHA string `json:"commit_sha,omitempty"`
	Status    string `json:"status"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

type staticPreviewsResponse struct {
	Previews []staticPreviewEntry `json:"previews"`
}

func previewEntries(ps []store.PreviewEnvironment) []pagespreviews.Entry {
	out := make([]pagespreviews.Entry, 0, len(ps))
	for _, p := range ps {
		t, err := time.Parse(time.RFC3339Nano, p.UpdatedAt)
		if err != nil {
			t, _ = time.Parse(time.RFC3339, p.CreatedAt)
		}
		out = append(out, pagespreviews.Entry{
			Branch: p.Branch, PRNumber: p.PRNumber, PreviewApp: p.PreviewAppID, HeadSHA: p.HeadSHA,
			Status: p.Status, Domain: p.Domain, UpdatedAt: t,
		})
	}
	return out
}

// handleListStaticSitePreviews handles GET /api/v1/static-sites/{name}/previews.
func (rt *Router) handleListStaticSitePreviews(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok || rt.previewEnvironments == nil {
		writeError(w, http.StatusNotImplemented, "branch previews are not available on this control plane")
		return
	}
	site, err := getter.GetStaticSite(ctx, name)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: list static previews: lookup", err, slog.String("name", name))
		return
	}
	if site == nil {
		writeError(w, http.StatusNotFound, "static site not found")
		return
	}
	all, err := rt.previewEnvironments.ListPreviewEnvironmentsByApp(ctx, name)
	if err != nil {
		rt.internalError(w, "api: list static previews: list", err, slog.String("name", name))
		return
	}
	out := staticPreviewsResponse{Previews: []staticPreviewEntry{}}
	for _, e := range pagespreviews.LatestPerBranch(previewEntries(all)) {
		host := e.Domain
		if host == "" {
			host, _ = rt.autoHost(ctx, e.PreviewApp)
		}
		entry := staticPreviewEntry{
			Branch: e.Branch, PRNumber: e.PRNumber, Site: e.PreviewApp, CommitSHA: e.HeadSHA,
			Status: e.Status, UpdatedAt: e.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if host != "" {
			entry.URL = rt.publicHTTPSURL(ctx, host)
		}
		out.Previews = append(out.Previews, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// prunePagesPreviews removes the oldest previews of appName beyond the retention
// setting, sparing keep (the preview just published).
func (rt *Router) prunePagesPreviews(ctx context.Context, appName, keep string) {
	if rt.runtimeSettings == nil || rt.previewEnvironments == nil {
		return
	}
	limit := rt.runtimeSettings.Limit(ctx, runtimesettings.KeyPagesPreviewMaxPerSite)
	all, err := rt.previewEnvironments.ListPreviewEnvironmentsByApp(ctx, appName)
	if err != nil {
		rt.logger.Warn("api: prune previews: list failed", slog.String("name", appName), slog.String("error", err.Error()))
		return
	}
	byApp := map[string]store.PreviewEnvironment{}
	for _, p := range all {
		byApp[p.PreviewAppID] = p
	}
	for _, v := range pagespreviews.Prune(previewEntries(all), limit, keep) {
		if status, msg := rt.teardownPreviewRecordReason(ctx, byApp[v.PreviewApp], "This preview was removed to stay within the preview limit."); status >= 300 {
			rt.logger.Warn("api: prune previews: teardown failed", slog.String("preview", v.PreviewApp), slog.String("message", msg))
		}
	}
}
