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
	RepoURL        string `json:"repo_url"`
	Ref            string `json:"ref,omitempty"`
	BaseDirectory  string `json:"base_directory,omitempty"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BuildType      string `json:"build_type,omitempty"`
	Name           string `json:"name,omitempty"`
	Token          string `json:"token,omitempty"`
}

// handleAnalyzeImport handles POST /api/v1/imports/analyze: reads a bounded
// set of files from the repository through the provider API (never a clone
// onto the host for GitHub, GitLab and Gitea) and returns pre-filled
// suggestions with evidence. Nothing is created.
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
		branches = []string{"main", "master"}
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
		writeError(w, http.StatusUnprocessableEntity, "this host has no supported file API; connect a GitHub, GitLab or Gitea provider, or import a compose file or image instead")
		return
	}
	rt.writeDetectError(w, "", req.RepoURL, lastErr)
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
	u, err := url.Parse(repoURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	if rt.gitlabApp != nil && rt.gitlabAppSecrets != nil {
		if conn, tok, err := rt.gitlabAccessToken(ctx); err == nil && hostOf(conn.InstanceURL) == host {
			return tok
		}
	}
	if rt.giteaApp != nil && rt.giteaAppSecrets != nil {
		if conn, tok, err := rt.giteaAccessToken(ctx); err == nil && hostOf(conn.InstanceURL) == host {
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
