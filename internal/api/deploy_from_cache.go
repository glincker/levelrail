package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// deployFromCacheRequest is POST /api/v1/apps/{name}/deploys/from-cache's body.
// Commit defaults to the commit of the app's last successful source deploy.
type deployFromCacheRequest struct {
	Commit string `json:"commit,omitempty"`
	freezeOverride
}

// handleDeployFromCache rebuilds the app from the local source cache without
// contacting the git host, so a deploy still works during a host outage. It
// goes through the ordinary build trigger, so queueing, freeze windows and
// attempt history behave the same.
func (rt *Router) handleDeployFromCache(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in deployFromCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deploy from cache: load git source failed", err)
		return
	}
	commit := in.Commit
	if commit == "" {
		if commit = rt.lastGoodSourceCommit(r.Context(), name); commit == "" {
			writeError(w, http.StatusUnprocessableEntity, "no earlier successful source deploy to redeploy; pass a commit")
			return
		}
	}
	body, err := json.Marshal(triggerBuildRequest{
		RepoURL:        gs.RepoURL,
		Ref:            commit,
		Build:          triggerBuildBuildInput{Type: gs.BuildType, Path: gs.BuildPath, BaseDirectory: gs.BaseDirectory},
		FromCache:      true,
		freezeOverride: in.freezeOverride,
	})
	if err != nil {
		rt.internalError(w, "api: deploy from cache: encode build request failed", err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	rt.handleTriggerBuild(w, r)
}

// lastGoodSourceCommit is the commit of name's newest successful deploy that
// was built from source, empty when there is none.
func (rt *Router) lastGoodSourceCommit(ctx context.Context, name string) string {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.logger.Warn("api: deploy from cache: list attempts failed", slog.String("name", name), slog.String("error", err.Error()))
		return ""
	}
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded || a.CommitSHA == "" {
			continue
		}
		switch a.Source {
		case store.DeployAttemptSourceWebhook, store.DeployAttemptSourceManual, store.DeployAttemptSourceSchedule, store.DeployAttemptSourcePoll:
			return a.CommitSHA
		}
	}
	return ""
}
