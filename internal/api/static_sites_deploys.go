package api

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticDeployEntry is one deploy of a Pages site with its own immutable URL.
type staticDeployEntry struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	// URL is empty once the deploy's files are pruned or automatic hostnames are off.
	URL string `json:"url,omitempty"`
	// Live marks the deploy the site's own domains currently serve.
	Live bool `json:"live"`
	// Promotable is true when promote would succeed: files retained and not already live.
	Promotable bool `json:"promotable"`
}

type staticDeploysResponse struct {
	Deploys []staticDeployEntry `json:"deploys"`
}

// handleListStaticSiteDeploys handles GET /api/v1/static-sites/{name}/deploys.
func (rt *Router) handleListStaticSiteDeploys(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok || rt.deployAttempts == nil {
		writeError(w, http.StatusNotImplemented, "static site deploys are not available on this control plane")
		return
	}
	site, err := getter.GetStaticSite(ctx, name)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: list static deploys: lookup", err, slog.String("name", name))
		return
	}
	if site == nil {
		writeError(w, http.StatusNotFound, "static site not found")
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.internalError(w, "api: list static deploys: attempts", err, slog.String("name", name))
		return
	}
	svcDir := filepath.Dir(filepath.Clean(site.RootDir))
	retained := map[string]string{}
	if filepath.Base(svcDir) == name {
		for _, d := range staticroutes.RetainedDeploys(attempts, svcDir) {
			retained[d.Attempt.ID] = d.Dir
		}
	}
	fallback, _ := rt.autoHost(ctx, name)
	out := staticDeploysResponse{Deploys: []staticDeployEntry{}}
	for _, a := range attempts {
		e := staticDeployEntry{
			ID: a.ID, Status: a.Status, Source: a.Source, CommitSHA: a.CommitSHA,
			StartedAt: a.StartedAt.UTC().Format(time.RFC3339),
		}
		if a.FinishedAt != nil {
			e.FinishedAt = a.FinishedAt.UTC().Format(time.RFC3339)
		}
		if dir, ok := retained[a.ID]; ok {
			e.Live = filepath.Clean(site.RootDir) == dir
			e.Promotable = !e.Live
			if host := staticroutes.DeployHost(fallback, a.ID); host != "" {
				e.URL = rt.publicHTTPSURL(ctx, host)
			}
		}
		out.Deploys = append(out.Deploys, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePromoteStaticSiteDeploy handles POST /api/v1/static-sites/{name}/deploys/{deployId}/promote.
// It is the rollback path under another name: it points the live site at that
// deploy's retained files after re-checking their content digest.
func (rt *Router) handlePromoteStaticSiteDeploy(w http.ResponseWriter, r *http.Request) {
	target, ok := rt.lookupAppAttempt(w, r, AbilityDeploy)
	if !ok {
		return
	}
	if !isStaticAttempt(*target) {
		writeError(w, http.StatusConflict, "this deploy has no published files to promote")
		return
	}
	if _, ok := rt.freezeGate(w, r, target.ServiceName, freezeOverride{}); !ok {
		return
	}
	status, msg, cdnWarning := rt.rollbackStatic(r.Context(), *target)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "promoted", "deploy_id": target.ID, "cdn_warning": cdnWarning})
}
