package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/pathfilter"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxDeployPathGlobs = 50

// deployPathFilter is the path filter a connected source applies to pushes.
func deployPathFilter(gs store.GitSource) pathfilter.Filter {
	return pathfilter.Filter{Paths: gs.DeployPaths, Ignore: gs.DeployPathsIgnore}
}

type gitDeploySettings struct {
	DeployPaths       []string `json:"deploy_paths"`
	DeployPathsIgnore []string `json:"deploy_paths_ignore"`
	ReportStatus      bool     `json:"report_status"`
}

type setGitDeploySettingsRequest struct {
	DeployPaths       *[]string `json:"deploy_paths,omitempty"`
	DeployPathsIgnore *[]string `json:"deploy_paths_ignore,omitempty"`
	ReportStatus      *bool     `json:"report_status,omitempty"`
}

func toGitDeploySettings(gs store.GitSource) gitDeploySettings {
	return gitDeploySettings{
		DeployPaths:       nonNilPaths(gs.DeployPaths),
		DeployPathsIgnore: nonNilPaths(gs.DeployPathsIgnore),
		ReportStatus:      gs.ReportStatus,
	}
}

func nonNilPaths(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// handleSetGitDeploySettings updates the push path filters and the status
// reporting flag of an app's connected source. Omitted fields keep their
// current value.
func (rt *Router) handleSetGitDeploySettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	var req setGitDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeployPaths == nil && req.DeployPathsIgnore == nil && req.ReportStatus == nil {
		writeError(w, http.StatusBadRequest, "deploy_paths, deploy_paths_ignore or report_status is required")
		return
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.logger.Error("api: load git source for deploy settings failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	next := toGitDeploySettings(*gs)
	if req.DeployPaths != nil {
		next.DeployPaths = *req.DeployPaths
	}
	if req.DeployPathsIgnore != nil {
		next.DeployPathsIgnore = *req.DeployPathsIgnore
	}
	if req.ReportStatus != nil {
		next.ReportStatus = *req.ReportStatus
	}
	if err := validateDeployPaths(next); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.gitSources.SetGitSourceDeploySettings(ctx, name, next.DeployPaths, next.DeployPathsIgnore, next.ReportStatus); err != nil {
		rt.logger.Error("api: set git deploy settings failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.logger.Info("api: git deploy settings updated", slog.String("name", name), slog.Int("paths", len(next.DeployPaths)), slog.Int("paths_ignore", len(next.DeployPathsIgnore)), slog.Bool("report_status", next.ReportStatus))
	writeJSON(w, http.StatusOK, next)
}

func validateDeployPaths(s gitDeploySettings) error {
	if len(s.DeployPaths) > maxDeployPathGlobs || len(s.DeployPathsIgnore) > maxDeployPathGlobs {
		return fmt.Errorf("at most %d globs per list", maxDeployPathGlobs)
	}
	return pathfilter.Filter{Paths: s.DeployPaths, Ignore: s.DeployPathsIgnore}.Validate()
}

// skipPushForPaths applies the source's path filter to a push. It returns a
// non-empty message when the push should not deploy. Unknown changed files
// (no list in the payload, and a failed forge lookup) let the push through.
func (rt *Router) skipPushForPaths(ctx context.Context, name string, gs store.GitSource, before, after string, inline []string) string {
	f := deployPathFilter(gs)
	if f.IsZero() {
		return ""
	}
	files := inline
	if len(files) == 0 {
		fetched, err := rt.changedFilesFn(name, changeQuery{Base: before, Head: after})(ctx)
		if err != nil {
			rt.logger.Warn("api: git push webhook: changed files unavailable, deploying unfiltered", slog.String("name", name), slog.String("error", err.Error()))
		}
		files = fetched
	}
	res := f.Apply(files)
	if res.Run {
		return ""
	}
	return fmt.Sprintf("ignored: %s\n", res.Reason)
}
