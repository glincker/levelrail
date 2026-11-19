package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// EnvPagesDeployIDWait bounds how long the create call waits for the first
// deploy attempt to exist so it can return its id (a Go duration).
const EnvPagesDeployIDWait = "APP_PAGES_DEPLOY_ID_WAIT"

const defaultPagesDeployIDWait = 3 * time.Second

// staticSitePlaceholderImage is the image a Pages app holds until its first publish.
func staticSitePlaceholderImage(name string) string { return name + staticSitePendingTag }

// staticSitePort and staticSitePendingTag match what the dashboard's drop
// publish creates: a static site has no container, so the app is a placeholder.
const (
	staticSitePort       = 80
	staticSitePendingTag = ":pending"
)

type createStaticSiteRequest struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains,omitempty"`
	// Git connects a repository as the source in the same call.
	Git *createStaticSiteGit `json:"git,omitempty"`
	// Deploy starts the first build right after connecting; needs Git.
	Deploy bool `json:"deploy,omitempty"`
	// SourceURL is an https .zip or .tar.gz published as the first deploy.
	SourceURL string `json:"source_url,omitempty"`
}

type createStaticSiteGit struct {
	RepoURL      string `json:"repo_url"`
	Branch       string `json:"branch,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	BuildCommand string `json:"build_command,omitempty"`
	OutputDir    string `json:"output_dir,omitempty"`
	// DeployOnPush nil or true means trigger_mode push, false means manual.
	DeployOnPush *bool `json:"deploy_on_push,omitempty"`
}

// createStaticSiteResponse is the site detail plus the git source and first deploy id.
type createStaticSiteResponse struct {
	staticSiteDetail
	GitSource *gitSourceResource `json:"git_source,omitempty"`
	DeployID  string             `json:"deploy_id,omitempty"`
}

// staticGitPlan is the validated git part of a create request.
type staticGitPlan struct {
	params connectGitSourceParams
	conn   string
}

// planStaticGit validates the git object before anything is created.
func (rt *Router) planStaticGit(w http.ResponseWriter, r *http.Request, name string, g *createStaticSiteGit) (*staticGitPlan, bool) {
	bad := func(msg string) (*staticGitPlan, bool) {
		writeError(w, http.StatusBadRequest, msg)
		return nil, false
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return nil, false
	}
	if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusNotFound, "app not found"), appRef(name)) {
		return nil, false
	}
	repo := strings.TrimSpace(g.RepoURL)
	if repo == "" {
		return bad("git.repo_url is required")
	}
	if err := requireHTTPOrHTTPSScheme(repo); err != nil && !gitsshkey.IsSSHURL(repo) {
		return bad("git.repo_url must use http, https or ssh")
	}
	cmd, out := strings.TrimSpace(g.BuildCommand), strings.TrimSpace(g.OutputDir)
	if err := spec.ValidateStaticOverrides(cmd, out, spec.StaticBuildCommandMaxFromEnv(os.LookupEnv)); err != nil {
		return bad(err.Error())
	}
	out, nerr := spec.NormalizeRepoPath("output_dir", out)
	if nerr != nil {
		return bad(nerr.Error())
	}
	p := connectGitSourceParams{RepoURL: repo, Branch: strings.TrimSpace(g.Branch), BuildType: spec.BuildStatic, BuildCommand: cmd, TriggerMode: spec.TriggerModePush}
	if p.Branch == "" {
		p.Branch = webhook.DefaultBranch
	}
	if g.DeployOnPush != nil && !*g.DeployOnPush {
		p.TriggerMode = spec.TriggerModeManual
	}
	// With a command the output dir is the publish dir; without one it is the served subdirectory.
	if cmd != "" {
		p.PublishDir = out
	} else {
		p.BuildPath = out
	}
	if err := validateGitSourceStaticBuild(name, p.BuildType, p.BuildPath, p.BuildCommand, p.PublishDir); err != nil {
		return bad(err.Error())
	}
	if g.AccountID != "" {
		conn, err := rt.gitConns.GetGitConnection(r.Context(), g.AccountID)
		if err != nil {
			return bad("git.account_id does not match a connected account")
		}
		if !repoOnConnectionInstance(repo, conn.InstanceURL) {
			return bad("git.repo_url must be on the connected account's host")
		}
	}
	return &staticGitPlan{params: p, conn: g.AccountID}, true
}

// startFirstStaticBuild starts the build in the background and waits briefly for its attempt id.
func (rt *Router) startFirstStaticBuild(ctx context.Context, name, branch string) string {
	release, ok := rt.markGitSourceDeployStarting(ctx, name)
	if !ok {
		return ""
	}
	began := time.Now().Add(-time.Second)
	bg := withStartRelease(context.WithoutCancel(ctx), release)
	go func() {
		defer release()
		if _, err := rt.triggerGitDeploy(bg, name, branch, store.DeployAttemptSourceManual); err != nil {
			rt.logger.Warn("api: static site first build failed", slog.String("name", name), slog.String("error", err.Error()))
		}
	}()
	wait := defaultPagesDeployIDWait
	if d, err := time.ParseDuration(os.Getenv(EnvPagesDeployIDWait)); err == nil && d > 0 {
		wait = d
	}
	if rt.deployAttempts == nil {
		return ""
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if list, err := rt.deployAttempts.ListDeployAttempts(ctx, name); err == nil && len(list) > 0 && list[0].StartedAt.After(began) {
			return list[0].ID
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

// rollbackStaticSite deletes the app a failed create just made so a retry with
// the same name does not hit a 409 on a half-configured site.
func (rt *Router) rollbackStaticSite(ctx context.Context, name string) {
	if _, err := rt.deleteApp(context.WithoutCancel(ctx), name); err != nil {
		rt.logger.Warn("api: static site rollback failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// handleCreateStaticSite handles POST /api/v1/static-sites. It creates the
// backing app through handleCreateApp so every create rule (names, quotas,
// domains, projects, team scoping) applies unchanged.
func (rt *Router) handleCreateStaticSite(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req createStaticSiteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.SourceURL != "" {
		if req.Git != nil || req.Deploy {
			writeError(w, http.StatusBadRequest, "source_url cannot be combined with git or deploy")
			return
		}
		if rt.builder == nil {
			writeError(w, http.StatusNotImplemented, "manual build trigger is not configured on this control plane")
			return
		}
		if !rt.authorizeRefs(w, r, AbilityDeploy, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.Name)) {
			return
		}
		if _, perr := parsePagesSourceURL(req.SourceURL); perr != nil {
			writeError(w, perr.status, perr.msg)
			return
		}
	}
	var plan *staticGitPlan
	if req.Git != nil {
		var ok bool
		if plan, ok = rt.planStaticGit(w, r, req.Name, req.Git); !ok {
			return
		}
	} else if req.Deploy {
		writeError(w, http.StatusBadRequest, "deploy needs a git source")
		return
	}
	body, err := json.Marshal(appResource{
		Name:    req.Name,
		Image:   staticSitePlaceholderImage(req.Name),
		Port:    staticSitePort,
		Domains: req.Domains,
	})
	if err != nil {
		rt.internalError(w, "api: create static site: encode", err)
		return
	}
	inner := r.Clone(r.Context())
	inner.Body = io.NopCloser(strings.NewReader(string(body)))
	inner.ContentLength = int64(len(body))

	rec := httptest.NewRecorder()
	rt.handleCreateApp(rec, inner)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	domains := req.Domains
	var created appResource
	if json.Unmarshal(rec.Body.Bytes(), &created) == nil && created.Domains != nil {
		domains = created.Domains
	}
	detail := staticSiteDetail{Name: req.Name, BuildType: "static", Domains: nonNilStrings(domains)}
	if req.SourceURL != "" {
		// Staged only now, after every create check passed, so a refused create leaves no archive behind.
		hash, fresh, serr := rt.stagePagesSourceURL(r.Context(), req.Name, req.SourceURL)
		if serr != nil {
			if rec.Code == http.StatusCreated {
				rt.rollbackStaticSite(r.Context(), req.Name)
			}
			writeError(w, serr.status, serr.msg)
			return
		}
		id, brec := rt.deployStaticFromURL(r, req.Name, hash)
		if brec.Code >= 300 {
			if fresh {
				rt.removeStagedPagesSource(req.Name, hash)
			}
			if rec.Code == http.StatusCreated {
				rt.rollbackStaticSite(r.Context(), req.Name)
			}
			for k, v := range brec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(brec.Code)
			_, _ = w.Write(brec.Body.Bytes())
			return
		}
		rt.auditPagesURLDeploy(r, req.Name, req.SourceURL)
		writeJSON(w, http.StatusCreated, createStaticSiteResponse{staticSiteDetail: detail, DeployID: id})
		return
	}
	if plan == nil {
		writeJSON(w, http.StatusCreated, detail)
		return
	}
	ctx := withRequestOrigin(r.Context(), r)
	result, err := rt.connectGitSource(ctx, req.Name, plan.params)
	if err == nil && plan.conn != "" {
		err = rt.gitConns.SetGitSourceConnection(ctx, req.Name, plan.conn)
	}
	if err != nil {
		// Only an app this request created is removed; a 409 never reaches here.
		if rec.Code == http.StatusCreated {
			rt.rollbackStaticSite(ctx, req.Name)
		}
		if errors.Is(err, errGitSourceKindConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		rt.internalError(w, "api: create static site: connect git source failed", err, slog.String("name", req.Name))
		return
	}
	out := createStaticSiteResponse{staticSiteDetail: detail, GitSource: &result.Resource}
	if req.Deploy && rt.builder != nil {
		out.DeployID = rt.startFirstStaticBuild(ctx, req.Name, plan.params.Branch)
	}
	writeJSON(w, http.StatusCreated, out)
}
