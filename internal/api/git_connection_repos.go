package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

type gitConnBranchResource struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

// handleListGitConnectionBranches handles
// GET /api/v1/git-connections/{id}/branches?repo=: a project id for GitLab,
// owner/name for Bitbucket and Gitea. AbilityReadSensitive like the repo listing.
func (rt *Router) handleListGitConnectionBranches(w http.ResponseWriter, r *http.Request) {
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo == "" {
		writeError(w, http.StatusBadRequest, "repo is required")
		return
	}
	ctx := r.Context()
	conn, token, err := rt.gitConnToken(ctx, r.PathValue("id"))
	if err != nil {
		rt.writeGitConnTokenError(w, "api: mint git access token for branch listing failed", err)
		return
	}
	adapter, _ := rt.gitAdapter(conn.Provider)
	branches, err := adapter.branches(ctx, conn, token, repo)
	if err != nil {
		rt.logger.Error("api: list git repo branches failed", slog.String("connection", conn.ID), slog.String("repo", repo), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list branches from "+adapter.label)
		return
	}
	out := make([]gitConnBranchResource, 0, len(branches))
	for _, b := range branches {
		out = append(out, gitConnBranchResource(b))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUseGitConnectionRepoAsSource handles POST /api/v1/git-connections/{id}/use-as-source:
// the picked repository becomes the app's git source, the webhook is
// registered through the same connection, and the source records which
// connection it came from.
//
// Known gap: no deploy token is set from this path, so a private repository
// builds only once a token is pasted into the app's git-source card.
func (rt *Router) handleUseGitConnectionRepoAsSource(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	req, buildType, triggerMode, ok := rt.decodeUseAsSourceRequest(w, r, "api: use git connection repo as source")
	if !ok {
		return
	}
	repo := strings.TrimSpace(req.Repo)
	if repo == "" {
		writeError(w, http.StatusBadRequest, "repo is required")
		return
	}
	ctx := r.Context()
	conn, token, err := rt.gitConnToken(ctx, r.PathValue("id"))
	if err != nil {
		rt.writeGitConnTokenError(w, "api: mint git access token for use-as-source failed", err)
		return
	}
	adapter, _ := rt.gitAdapter(conn.Provider)
	info, err := adapter.lookup(ctx, conn, token, repo)
	if err != nil {
		rt.logger.Error("api: look up git repo failed", slog.String("connection", conn.ID), slog.String("repo", repo), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to look up the repository on "+adapter.label)
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = info.DefaultBranch
	}
	if branch == "" {
		branch = webhook.DefaultBranch
	}
	result, err := rt.connectGitSource(ctx, req.AppName, connectGitSourceParams{
		RepoURL: info.CloneURL, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath, BaseDirectory: req.BaseDirectory,
		TriggerMode: triggerMode,
	})
	if err != nil {
		rt.internalError(w, "api: use git connection repo as source: connect git source failed", err, slog.String("app_name", req.AppName), slog.String("connection", conn.ID))
		return
	}
	if err := rt.gitConns.SetGitSourceConnection(ctx, req.AppName, conn.ID); err != nil {
		rt.internalError(w, "api: record git source connection failed", err, slog.String("app_name", req.AppName), slog.String("connection", conn.ID))
		return
	}
	result.Resource.ConnectionID = conn.ID

	if _, err := rt.controlPlaneBaseURL(ctx); err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			rt.recordWebhookRegistration(ctx, req.AppName, errWebhookNoPublicURL)
			writeError(w, http.StatusConflict, "git source connected, but set a primary domain in ingress settings before "+adapter.label+" can reach a webhook here")
			return
		}
		rt.internalError(w, "api: get base url for webhook registration failed", err, slog.String("app_name", req.AppName))
		return
	}
	webhookSecret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(req.AppName), gitSourceSecretKey)
	if err != nil {
		rt.internalError(w, "api: resolve git source webhook secret failed", err, slog.String("app_name", req.AppName))
		return
	}
	if err := adapter.webhook(ctx, conn, token, repo, result.Resource.WebhookURL, webhookSecret); err != nil {
		rt.recordWebhookRegistration(ctx, req.AppName, err)
		rt.logger.Error("api: register git repo webhook failed", slog.String("connection", conn.ID), slog.String("repo", repo), slog.String("app_name", req.AppName), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "git source connected, but registering the webhook on "+adapter.label+" failed; add it manually")
		return
	}
	rt.recordWebhookRegistration(ctx, req.AppName, nil)
	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: git repo connected as git source", slog.String("app_name", req.AppName), slog.String("connection", conn.ID), slog.String("repo", repo), slog.Bool("creating", result.Creating))
	writeJSON(w, status, result.Resource)
}
