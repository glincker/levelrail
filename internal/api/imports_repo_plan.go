package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/repoplan"
)

// EnvImportPlanConcurrency bounds how many packages are analyzed at once.
const EnvImportPlanConcurrency = "APP_WIZARD_PLAN_CONCURRENCY"

const defaultImportPlanConcurrency = 4

// importRepoPlanRequest is POST /api/v1/imports/repo/plan's body. Token is a
// one-shot read token for a private repository, used for this call only.
type importRepoPlanRequest struct {
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
	Token         string `json:"token,omitempty"`
}

// handleImportRepoPlan returns one proposed app per workspace package of a
// repository. It reads through the provider API only and creates nothing.
func (rt *Router) handleImportRepoPlan(w http.ResponseWriter, r *http.Request) {
	var req importRepoPlanRequest
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
	if req.Branch == "" {
		req.Branch = pinnedRef
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
	branches := []string{strings.TrimSpace(req.Branch)}
	if branches[0] == "" {
		branches = rt.analyzeBranchCandidates(ctx, open, req.RepoURL, token, hosts)
	}

	var lastErr error
	for _, branch := range branches {
		src, err := open(ctx, req.RepoURL, branch, token, hosts)
		if err != nil {
			lastErr = err
			break
		}
		cached := newCachedSource(src)
		layout, err := repolayout.Detect(ctx, cached)
		if err == nil {
			existing, lerr := rt.existingAppNames(ctx)
			if lerr != nil {
				rt.internalError(w, "api: import repo plan: list apps failed", lerr)
				return
			}
			writeJSON(w, http.StatusOK, rt.buildRepoPlan(ctx, cached, req, branch, layout, existing))
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

func (rt *Router) existingAppNames(ctx context.Context) (map[string]bool, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(svcs))
	for _, s := range svcs {
		out[s.Name] = true
	}
	return out, nil
}

func (rt *Router) buildRepoPlan(ctx context.Context, src repolayout.Source, req importRepoPlanRequest, branch string, layout repolayout.Result, existing map[string]bool) repoplan.Plan {
	base := strings.Trim(req.BaseDirectory, "/ ")
	var pkgs []repoplan.Package
	for _, p := range layout.Packages {
		if base != "" && p.Dir != base && !strings.HasPrefix(p.Dir, base+"/") {
			continue
		}
		pkgs = append(pkgs, repoplan.Package{PackageName: p.Name, Dir: p.Dir, Dockerfile: p.Dockerfile, DependsOn: p.DependsOn, WatchPaths: p.WatchPaths})
	}
	detect := detectPackages(ctx, pkgs, importPlanConcurrency(), func(p repoplan.Package) repoplan.Detection {
		a, err := appwizard.Analyze(ctx, src, appwizard.Options{
			RepoURL: req.RepoURL, Branch: branch, BaseDirectory: p.Dir, DockerfilePath: p.Dockerfile,
			DefaultVersion: defaultEngineVersion,
		})
		if err != nil {
			return repoplan.Detection{Failed: true}
		}
		kind := repoplan.KindBuildpack
		switch a.Build.Type {
		case "dockerfile":
			kind = repoplan.KindDockerfile
		case "static":
			kind = repoplan.KindStatic
		}
		return repoplan.Detection{Port: a.Port.Value, PortSource: a.Port.Evidence, Kind: kind}
	})
	return repoplan.Build(repoplan.Input{
		RepoURL: req.RepoURL, Branch: branch, BaseDirectory: base, Tool: firstOf(layout.Tools),
		Packages: pkgs, Found: layout.PackagesTotal, Cap: repolayout.PackageCap(), Truncated: layout.Truncated,
		Detect: detect, Existing: existing, MaxNameLen: appwizard.MaxAppNameLen,
		NameFor: func(dir string) string { return appwizard.PackageAppName(req.RepoURL, dir) },
	})
}

func firstOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func importPlanConcurrency() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvImportPlanConcurrency))); err == nil && n > 0 {
		return n
	}
	return defaultImportPlanConcurrency
}

// detectPackages runs fn for every package with at most limit in flight.
func detectPackages(ctx context.Context, pkgs []repoplan.Package, limit int, fn func(repoplan.Package) repoplan.Detection) map[string]repoplan.Detection {
	out := make(map[string]repoplan.Detection, len(pkgs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)
	for _, p := range pkgs {
		if ctx.Err() != nil {
			mu.Lock()
			out[p.Dir] = repoplan.Detection{Failed: true}
			mu.Unlock()
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			d := fn(p)
			mu.Lock()
			out[p.Dir] = d
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// cachedSource memoizes the tree listing and file reads so N per-package
// analyses cost one listing and one read per distinct file.
type cachedSource struct {
	src   repolayout.Source
	mu    sync.Mutex
	paths []string
	trunc bool
	have  bool
	reads map[string]cachedRead
}

type cachedRead struct {
	data []byte
	err  error
}

func newCachedSource(src repolayout.Source) *cachedSource {
	return &cachedSource{src: src, reads: map[string]cachedRead{}}
}

func (c *cachedSource) Paths(ctx context.Context) ([]string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.have {
		p, t, err := c.src.Paths(ctx)
		if err != nil {
			return nil, false, err
		}
		c.paths, c.trunc, c.have = p, t, true
	}
	return c.paths, c.trunc, nil
}

func (c *cachedSource) Read(ctx context.Context, p string, limit int) ([]byte, error) {
	c.mu.Lock()
	if r, ok := c.reads[p]; ok {
		c.mu.Unlock()
		return r.data, r.err
	}
	c.mu.Unlock()
	data, err := c.src.Read(ctx, p, limit)
	c.mu.Lock()
	c.reads[p] = cachedRead{data, err}
	c.mu.Unlock()
	return data, err
}
