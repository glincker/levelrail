package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/repolayout"
)

// repoPlanRequest is POST /api/v1/apps/repo-plan's body. Token is a one-shot
// read token for a private repository, used for this call only.
type repoPlanRequest struct {
	RepoURL       string `json:"repo_url"`
	Ref           string `json:"ref,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
	Token         string `json:"token,omitempty"`
}

// handleRepoPlan returns a reviewable plan (stack, draft spec, masked env
// names, ports, warnings) for a repository. It reads through the provider
// API like the wizard detect route and applies nothing.
func (rt *Router) handleRepoPlan(w http.ResponseWriter, r *http.Request) {
	var req repoPlanRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, analyzeBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	pinnedURL, pinnedRef, pinnedDir := normalizePastedRepoURL(req.RepoURL)
	req.RepoURL = pinnedURL
	if req.Ref == "" {
		req.Ref = pinnedRef
	}
	if req.BaseDirectory == "" {
		req.BaseDirectory = pinnedDir
	}
	if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil {
		writeError(w, http.StatusBadRequest, "repo_url must be a public http or https git URL")
		return
	}
	ctx := r.Context()
	token := req.Token
	if token == "" {
		token = rt.analysisToken(ctx, req.RepoURL)
	}
	open := rt.repoLayoutSource
	if open == nil {
		open = apiOnlyRepoLayoutSource
	}
	hosts := rt.repoLayoutHosts(ctx)
	branches := []string{strings.TrimSpace(req.Ref)}
	if branches[0] == "" {
		branches = rt.analyzeBranchCandidates(ctx, open, req.RepoURL, token, hosts)
	}
	base := rt.appsBaseDomain(ctx)
	var lastErr error
	for _, branch := range branches {
		src, err := open(ctx, req.RepoURL, branch, token, hosts)
		if err != nil {
			lastErr = err
			break
		}
		a, err := appwizard.Analyze(ctx, src, appwizard.Options{
			RepoURL: req.RepoURL, Branch: branch, BaseDirectory: strings.Trim(req.BaseDirectory, "/ "),
			AppsBaseDomain: base, DomainFor: appBaseHost, DefaultVersion: defaultEngineVersion,
		})
		if err == nil {
			writeJSON(w, http.StatusOK, appwizard.BuildPlanView(a, appwizard.PlanNext{Method: http.MethodPost, Path: "/api/v1/imports/repo/apply"}))
			return
		}
		lastErr = err
		var se *repolayout.StatusError
		if !errors.As(err, &se) || se.Status != http.StatusNotFound {
			break
		}
	}
	if errors.Is(lastErr, repolayout.ErrUnsupportedHost) {
		writeError(w, http.StatusUnprocessableEntity, "this host has no supported file API; connect a GitHub, GitLab, Gitea, Forgejo or Bitbucket Server provider")
		return
	}
	rt.writeDetectError(w, "", req.RepoURL, lastErr)
}
