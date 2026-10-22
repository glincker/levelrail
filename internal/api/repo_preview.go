package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repopreview"
)

type repoPreviewState struct {
	once    sync.Once
	builder *repopreview.Builder
}

type repoPreviewRequest struct {
	RepoURL   string `json:"repo_url"`
	Ref       string `json:"ref,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Token     string `json:"token,omitempty"`
}

func (rt *Router) repoPreviewBuilder() *repopreview.Builder {
	rt.repoPreview.once.Do(func() {
		rt.repoPreview.builder = &repopreview.Builder{
			Client: &repopreview.HTTPClient{HTTP: gitprovider.NewGuardedClient(), Hosts: rt.repoLayoutHosts},
			Extra: func(ctx context.Context) []string {
				h := rt.repoLayoutHosts(ctx)
				return []string{h.GitHub, h.GitLab, h.Gitea, h.BitbucketServer}
			},
			Analyze: rt.previewAnalyze,
		}
	})
	return rt.repoPreview.builder
}

func (rt *Router) previewAnalyze(ctx context.Context, repoURL, ref, token string) (*appwizard.Analysis, error) {
	open := rt.repoLayoutSource
	if open == nil {
		open = apiOnlyRepoLayoutSource
	}
	src, err := open(ctx, repoURL, ref, token, rt.repoLayoutHosts(ctx))
	if err != nil {
		return nil, err
	}
	return appwizard.Analyze(ctx, src, appwizard.Options{
		RepoURL: repoURL, Branch: ref, AppsBaseDomain: rt.appsBaseDomain(ctx), DomainFor: appBaseHost, DefaultVersion: defaultEngineVersion,
	})
}

func (rt *Router) previewToken(ctx context.Context, repoURL, accountID string) string {
	if accountID != "" {
		for _, p := range []string{gitProviderGitHub, gitProviderGitLab, gitProviderGitea, gitProviderForgejo, gitProviderBitbucketServer} {
			if _, tok, err := rt.gitTokenForRepo(ctx, p, repoURL, accountID); err == nil && tok != "" {
				return tok
			}
		}
		return ""
	}
	return rt.analysisToken(ctx, repoURL)
}

// handlePreviewRepo handles POST /api/v1/git-sources/preview. Host failures
// are answered 200 with a kind so the wizard can degrade instead of erroring.
func (rt *Router) handlePreviewRepo(w http.ResponseWriter, r *http.Request) {
	var req repoPreviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, analyzeBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	pinnedURL, pinnedRef, _ := normalizePastedRepoURL(req.RepoURL)
	if req.Ref == "" {
		req.Ref = pinnedRef
	}
	ctx := r.Context()
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = rt.previewToken(ctx, pinnedURL, strings.TrimSpace(req.AccountID))
	}
	res, err := rt.repoPreviewBuilder().Build(ctx, pinnedURL, strings.TrimSpace(req.Ref), token)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		slog.Warn("repo preview failed", "host", hostOf(pinnedURL), "error", err)
		writeError(w, http.StatusBadGateway, "the repository host did not answer in time")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
