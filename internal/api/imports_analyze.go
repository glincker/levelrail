package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

const analyzeBodyLimit = 16 << 10

// analyzeImportRequest is POST /api/v1/imports/analyze's body. Token is an
// optional one-shot deploy token for a private repository; it is used for
// this call only and never stored or logged.
type analyzeImportRequest struct {
	RepoURL       string `json:"repo_url"`
	Ref           string `json:"ref,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
	// Directory is an alias of BaseDirectory, used by the git-sources route.
	Directory      string `json:"directory,omitempty"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BuildType      string `json:"build_type,omitempty"`
	Name           string `json:"name,omitempty"`
	Token          string `json:"token,omitempty"`
}

// Reads a bounded set of files through the provider API, never a host clone, and returns suggestions with evidence.
// Creates nothing.
func (rt *Router) handleAnalyzeImport(w http.ResponseWriter, r *http.Request) {
	var req analyzeImportRequest
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
	if req.BaseDirectory == "" && req.Directory == "" {
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
	if req.BaseDirectory == "" {
		req.BaseDirectory = strings.Trim(req.Directory, "/ ")
	}
	base := rt.appsBaseDomain(ctx)
	var lastErr error
	for _, branch := range branches {
		src, err := open(ctx, req.RepoURL, branch, token, hosts)
		if err != nil {
			lastErr = err
			break
		}
		res, err := appwizard.Analyze(ctx, src, appwizard.Options{
			RepoURL: req.RepoURL, Branch: branch, BaseDirectory: req.BaseDirectory, DockerfilePath: req.DockerfilePath,
			BuildType: req.BuildType, Name: req.Name, AppsBaseDomain: base, DomainFor: appBaseHost, DefaultVersion: defaultEngineVersion,
		})
		if err == nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
		lastErr = err
		var se *repolayout.StatusError
		if !errors.As(err, &se) || se.Status != http.StatusNotFound {
			break
		}
	}
	if errors.Is(lastErr, repolayout.ErrUnsupportedHost) {
		writeError(w, http.StatusUnprocessableEntity, "this host has no supported file API; connect a GitHub, GitLab, Gitea, Forgejo or Bitbucket Server provider, or import a compose file or image instead")
		return
	}
	rt.writeDetectError(w, "", req.RepoURL, lastErr)
}

// analyzeBranchCandidates asks the provider for the default branch and keeps main then master
// only as the fallback for hosts that cannot answer.
func (rt *Router) analyzeBranchCandidates(ctx context.Context, open repoLayoutSourceFunc, repoURL, token string, hosts repolayout.Hosts) []string {
	fallback := []string{"main", "master"}
	probe, err := open(ctx, repoURL, "", token, hosts)
	if err != nil {
		return fallback
	}
	db, ok := probe.(repolayout.DefaultBrancher)
	if !ok {
		return fallback
	}
	branch, err := db.DefaultBranch(ctx)
	if err != nil || strings.TrimSpace(branch) == "" {
		return fallback
	}
	return []string{strings.TrimSpace(branch)}
}

// apiOnlyRepoLayoutSource never falls back to a clone: analysis reads only
// through a provider API, so untrusted code never reaches the host's disk.
func apiOnlyRepoLayoutSource(_ context.Context, repoURL, branch, token string, hosts repolayout.Hosts) (repolayout.Source, error) {
	return repolayout.NewAPISource(gitprovider.NewGuardedClient(), repoURL, branch, token, hosts)
}

func defaultEngineVersion(engine string) string {
	engines, err := store.SupportedDatabaseEngines()
	if err != nil {
		return ""
	}
	for _, e := range engines {
		if e.ID == engine {
			return e.DefaultVersion
		}
	}
	return ""
}

// analysisToken resolves a read credential for repoURL from the connected
// provider apps, or "" for a public repository.
func (rt *Router) analysisToken(ctx context.Context, repoURL string) string {
	if t := rt.tokenForRepo(ctx, "", repoURL, true); t != "" {
		return t
	}
	for _, p := range []string{gitProviderGitLab, gitProviderGitea, gitProviderForgejo, gitProviderBitbucketServer} {
		if _, tok, err := rt.gitTokenForRepo(ctx, p, repoURL, ""); err == nil {
			return tok
		}
	}
	return ""
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
