package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticSiteDeploy is the newest deploy attempt of a Pages site.
type staticSiteDeploy struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// staticSiteDetail is one Pages site: its domains, published size and latest deploy.
type staticSiteDetail struct {
	Name         string            `json:"name"`
	BuildType    string            `json:"build_type"`
	Domains      []string          `json:"domains"`
	Published    bool              `json:"published"`
	FileCount    int               `json:"file_count"`
	SizeBytes    int64             `json:"size_bytes"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
	LatestDeploy *staticSiteDeploy `json:"latest_deploy,omitempty"`
}

// handleGetStaticSite handles GET /api/v1/static-sites/{name}. A site that
// exists as an app but has not published yet is returned unpublished, so a
// freshly created Pages site is readable before its first deploy.
func (rt *Router) handleGetStaticSite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "static site not found"), staticSiteRef(name), appRef(name)) {
		return
	}
	out := staticSiteDetail{Name: name, BuildType: "static", Domains: []string{}}

	var site *store.StaticSite
	if getter, ok := rt.staticSites.(staticSiteGetter); ok {
		s, err := getter.GetStaticSite(r.Context(), name)
		if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.internalError(w, "api: get static site: lookup", err, slog.String("name", name))
			return
		}
		if err == nil {
			site = s
		}
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: get static site: app lookup", err, slog.String("name", name))
		return
	}
	if site == nil && errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "static site not found")
		return
	}
	if site != nil {
		out.Published = true
		out.Domains = nonNilStrings(site.Domains)
		if !site.UpdatedAt.IsZero() {
			out.UpdatedAt = site.UpdatedAt.UTC().Format(time.RFC3339)
		}
		if site.RootDir != "" {
			// A missing tree just reports zero size; the row is still the source of truth.
			if st, serr := deploy.HashStaticTree(site.RootDir); serr == nil {
				out.FileCount, out.SizeBytes = st.Files, st.Bytes
			}
		}
	} else if svc != nil {
		out.Domains = nonNilStrings(svc.Domains)
	}

	if rt.deployAttempts != nil {
		attempts, aerr := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
		if aerr != nil {
			rt.internalError(w, "api: get static site: deploys", aerr, slog.String("name", name))
			return
		}
		if len(attempts) > 0 {
			out.LatestDeploy = toStaticSiteDeploy(attempts[0])
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func toStaticSiteDeploy(a store.DeployAttempt) *staticSiteDeploy {
	d := &staticSiteDeploy{
		ID: a.ID, Status: a.Status, Source: a.Source, CommitSHA: a.CommitSHA,
		StartedAt: a.StartedAt.UTC().Format(time.RFC3339), Error: a.Error,
	}
	if a.FinishedAt != nil {
		d.FinishedAt = a.FinishedAt.UTC().Format(time.RFC3339)
	}
	return d
}
