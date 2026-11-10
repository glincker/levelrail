package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// Current source kinds an app can have before a git source is attached.
const (
	gitConnectSourceGit     = "git"
	gitConnectSourceImage   = "image"
	gitConnectSourceUpload  = "upload"
	gitConnectSourceNone    = "none"
	gitConnectWarnGroup     = "group_member"
	gitConnectWarnReplace   = "replaces_git_source"
	gitConnectWarnUploads   = "uploads_stop"
	gitConnectWarnNotBuilt  = "never_deployed"
	gitConnectWarnProtected = "protected_environment"
	// The protection state could not be read; deploy now refuses until it can.
	gitConnectWarnProtectedUnknown = "protection_unknown"
	gitConnectWarnConnNeedsRead    = "connection_needs_read_sensitive"
)

type gitSourcePlanRequest struct {
	RepoURL string `json:"repo_url,omitempty"`
	Branch  string `json:"branch,omitempty"`
	// ConnectionID picks the provider connection whose token reads a private repo.
	ConnectionID string `json:"connection_id,omitempty"`
}

// gitSourcePlanPreserved lists what a switch to git keeps; counts only, never values.
type gitSourcePlanPreserved struct {
	EnvVars      int      `json:"env_vars"`
	SecretEnv    int      `json:"secret_env"`
	Domains      []string `json:"domains"`
	Volumes      []string `json:"volumes"`
	Replicas     int      `json:"replicas"`
	RunningImage string   `json:"running_image,omitempty"`
}

type gitSourcePlanSuggestion struct {
	BuildType     string `json:"build_type"`
	BuildPath     string `json:"build_path,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
	BuildCommand  string `json:"build_command,omitempty"`
	PublishDir    string `json:"publish_dir,omitempty"`
	Framework     string `json:"framework,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type gitSourcePlanResponse struct {
	App           string                   `json:"app"`
	CurrentSource string                   `json:"current_source"`
	CurrentRepo   string                   `json:"current_repo,omitempty"`
	Static        bool                     `json:"static"`
	Branch        string                   `json:"branch,omitempty"`
	Preserved     gitSourcePlanPreserved   `json:"preserved"`
	Suggestion    *gitSourcePlanSuggestion `json:"suggestion,omitempty"`
	Detection     *repolayout.Result       `json:"detection,omitempty"`
	DetectError   string                   `json:"detect_error,omitempty"`
	Warnings      []string                 `json:"warnings"`
	Group         []gitSourcePlanMember    `json:"group"`
}

// handleGitSourcePlan handles POST /api/v1/apps/{name}/git-source/plan: a
// dry run of attaching a repository to an existing app, persists nothing.
func (rt *Router) handleGitSourcePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	var req gitSourcePlanRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: git source plan: load app failed", err, slog.String("name", name))
		return
	}
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	if req.RepoURL != "" {
		if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil && !gitsshkey.IsSSHURL(req.RepoURL) {
			writeError(w, http.StatusBadRequest, "repo_url must use http, https or ssh")
			return
		}
	}

	out := gitSourcePlanResponse{App: name, Warnings: []string{}, Group: []gitSourcePlanMember{}}
	out.Static = rt.isStaticSiteApp(ctx, name)
	out.Preserved = planPreserved(*svc)
	out.CurrentSource = gitConnectSourceImage
	switch {
	case spec.IsPendingImage(svc.Image) || svc.Image == "":
		out.CurrentSource = gitConnectSourceNone
		out.Warnings = append(out.Warnings, gitConnectWarnNotBuilt)
	case out.Static:
		out.CurrentSource = gitConnectSourceUpload
		out.Warnings = append(out.Warnings, gitConnectWarnUploads)
	}
	if gs, gerr := rt.gitSources.GetGitSource(ctx, name); gerr == nil {
		out.CurrentSource, out.CurrentRepo = gitConnectSourceGit, redactURLCredentials(gs.RepoURL)
		out.Warnings = append(out.Warnings, gitConnectWarnReplace)
	} else if !errors.Is(gerr, store.ErrGitSourceNotFound) {
		rt.internalError(w, "api: git source plan: load git source failed", gerr, slog.String("name", name))
		return
	}
	if svc.AppID != "" && svc.AppID != name {
		out.Warnings = append(out.Warnings, gitConnectWarnGroup)
	}
	canRead, _, verr := rt.callerAppVisibility(r)
	if verr != nil {
		// Fail closed: an unreadable scope lists no siblings.
		canRead = func(string) bool { return false }
	}
	out.Group = rt.planGroupMembers(ctx, *svc, canRead)
	if _, protected, perr := rt.protectedEnvironmentOf(ctx, name); perr != nil {
		out.Warnings = append(out.Warnings, gitConnectWarnProtectedUnknown)
	} else if protected {
		out.Warnings = append(out.Warnings, gitConnectWarnProtected)
	}

	if req.RepoURL != "" {
		out.Branch = strings.TrimSpace(req.Branch)
		if out.Branch == "" {
			out.Branch = webhook.DefaultBranch
		}
		// A deploy-only caller must not read private repos through the org GitHub App.
		allowAppToken := rt.callerHasAbility(r, AbilityReadSensitive) && gitHubAppTokenAllowed(ctx)
		connID := strings.TrimSpace(req.ConnectionID)
		if connID != "" && !allowAppToken {
			out.Warnings = append(out.Warnings, gitConnectWarnConnNeedsRead)
			connID = ""
		}
		res, derr := rt.detectLayoutFor(ctx, name, req.RepoURL, out.Branch, connID, allowAppToken)
		if derr != nil {
			out.DetectError = planDetectError(derr)
			rt.logger.Info("api: git source plan: detect failed", slog.String("name", name), slog.String("repo_url", redactURLCredentials(req.RepoURL)), slog.String("error", derr.Error()))
		} else {
			out.Detection = &res
			out.Suggestion = suggestFromLayout(res, out.Static)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func planPreserved(svc store.DesiredService) gitSourcePlanPreserved {
	p := gitSourcePlanPreserved{
		EnvVars:   len(svc.Env) + len(svc.VaultEnv) + len(svc.DatabaseEnv),
		SecretEnv: len(svc.SecretEnv),
		Domains:   append([]string{}, svc.Domains...),
		Volumes:   []string{},
		Replicas:  svc.Replicas,
	}
	for _, v := range svc.Volumes {
		p.Volumes = append(p.Volumes, v.ContainerPath)
	}
	if !spec.IsPendingImage(svc.Image) {
		p.RunningImage = svc.Image
	}
	return p
}

var errPlanSSHNotDetectable = errors.New("ssh repository urls cannot be read before connecting")

func planDetectError(err error) string {
	var se *repolayout.StatusError
	switch {
	case errors.Is(err, errPlanSSHNotDetectable):
		return "ssh_not_detectable"
	case errors.Is(err, repolayout.ErrUnsupportedHost):
		return "unsupported_host"
	case errors.As(err, &se) && se.Status == http.StatusNotFound:
		return "not_found"
	case errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden):
		return "access_denied"
	default:
		return "unreadable"
	}
}

// detectLayoutFor reads a repository that is not connected yet, with the
// credential the app would clone it with.
func (rt *Router) detectLayoutFor(ctx context.Context, name, repoURL, branch, connectionID string, allowAppToken bool) (repolayout.Result, error) {
	if gitsshkey.IsSSHURL(repoURL) {
		return repolayout.Result{}, errPlanSSHNotDetectable
	}
	if err := requireHTTPOrHTTPSScheme(repoURL); err != nil {
		return repolayout.Result{}, err
	}
	token := ""
	if connectionID != "" && allowAppToken {
		token = rt.planConnectionToken(ctx, repoURL, connectionID)
	}
	if token == "" {
		token = rt.tokenForRepo(ctx, name, repoURL, allowAppToken)
	}
	open := rt.repoLayoutSource
	if open == nil {
		open = planRepoLayoutSource
	}
	src, err := open(ctx, repoURL, branch, token, rt.repoLayoutHosts(ctx))
	if err != nil {
		return repolayout.Result{}, err
	}
	return repolayout.Detect(ctx, src)
}

// planRepoLayoutSource reads a caller-supplied URL through the provider APIs
// and the netguard client only: no clone fallback, which has no host guard or size cap.
func planRepoLayoutSource(_ context.Context, repoURL, branch, token string, hosts repolayout.Hosts) (repolayout.Source, error) {
	src, err := repolayout.NewAPISource(gitprovider.NewGuardedClient(), repoURL, branch, token, hosts)
	if err != nil {
		return nil, fmt.Errorf("open repository layout: %w", err)
	}
	return src, nil
}

// suggestFromLayout picks one build: a static app keeps serving static
// files, anything else takes the recommended suggestion.
func suggestFromLayout(res repolayout.Result, static bool) *gitSourcePlanSuggestion {
	if static {
		s := &gitSourcePlanSuggestion{BuildType: spec.BuildStatic}
		if res.StaticBuild != nil {
			s.BuildCommand, s.PublishDir, s.Framework = res.StaticBuild.Command, res.StaticBuild.PublishDir, res.StaticBuild.PresetName
		}
		return s
	}
	idx := slices.IndexFunc(res.Suggestions, func(s repolayout.Suggestion) bool { return s.Recommended })
	if idx < 0 && len(res.Suggestions) > 0 {
		idx = 0
	}
	if idx < 0 {
		return &gitSourcePlanSuggestion{BuildType: spec.BuildRailpack}
	}
	s := res.Suggestions[idx]
	return &gitSourcePlanSuggestion{BuildType: s.BuildType, BuildPath: s.DockerfilePath, BaseDirectory: s.BaseDirectory, Reason: s.ReasonCode}
}

func (rt *Router) isStaticSiteApp(ctx context.Context, name string) bool {
	if rt.staticSites == nil {
		return false
	}
	sites, err := rt.staticSites.ListStaticSites(ctx)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(sites, func(s store.StaticSite) bool { return s.Name == name })
}

type gitSourceDeployRequest struct {
	Branch string `json:"branch,omitempty"`
	freezeOverride
}

// markGitSourceDeployStarting holds the app busy from the click until the
// attempt row exists, so a second click cannot start a second build.
func (rt *Router) markGitSourceDeployStarting(ctx context.Context, name string) (release func(), ok bool) {
	rt.buildStartMu.Lock()
	defer rt.buildStartMu.Unlock()
	busy := rt.startingDeploys[name] > 0
	if !busy && rt.deploySafety != nil {
		busy = rt.deployBlocker(ctx, name)
	} else if !busy {
		busy = rt.hasRunningDeployAttempt(ctx, name)
	}
	if busy {
		return nil, false
	}
	rt.startingDeploys[name]++
	var once sync.Once
	return func() {
		once.Do(func() {
			rt.buildStartMu.Lock()
			defer rt.buildStartMu.Unlock()
			if rt.startingDeploys[name]--; rt.startingDeploys[name] <= 0 {
				delete(rt.startingDeploys, name)
			}
		})
	}, true
}

type gitSourceDeployResponse struct {
	App     string `json:"app"`
	Branch  string `json:"branch"`
	Started bool   `json:"started"`
}

// handleGitSourceDeploy handles POST /api/v1/apps/{name}/git-source/deploy:
// builds the connected branch head now. The running image serves until the
// build succeeds, and the previous deploy stays in history for rollback.
func (rt *Router) handleGitSourceDeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	var req gitSourceDeployRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if rt.builder == nil || rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git builds are not configured on this control plane")
		return
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app: connect one first (PUT /api/v1/apps/{name}/git-source)")
		return
	}
	if err != nil {
		rt.internalError(w, "api: git source deploy: load failed", err, slog.String("name", name))
		return
	}
	_, protected, perr := rt.protectedEnvironmentOf(ctx, name)
	if perr != nil {
		// Fail closed: an unknown protection state must not bypass approval.
		rt.logger.Error("api: git source deploy: check protected environment failed", slog.String("name", name), slog.String("error", perr.Error()))
		writeError(w, http.StatusServiceUnavailable, "could not check whether this app needs deploy approval, retry later")
		return
	}
	if protected {
		writeError(w, http.StatusConflict, "this app is in a protected environment: push to the branch so the deploy goes through approval")
		return
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.internalError(w, "api: git source deploy: load app failed", err, slog.String("name", name))
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = gs.Branch
	}
	// The busy check and the mark are one step; a refusal below drops the mark
	// so a 409 never leaves a freeze override entry for a deploy that never ran.
	release, ok := rt.markGitSourceDeployStarting(ctx, name)
	if !ok {
		writeError(w, http.StatusConflict, "a deploy for this app is already running or queued")
		return
	}
	freezeNote, ok := rt.freezeGate(w, r, name, req.freezeOverride)
	if !ok {
		release()
		return
	}
	if rt.refuseNodeCritical(ctx, w, existing.NodeID) {
		release()
		return
	}
	bg := withFreezeOverrideNote(withStartRelease(context.WithoutCancel(ctx), release), freezeNote)
	go func() {
		defer release()
		if _, err := rt.triggerGitDeploy(bg, name, branch, store.DeployAttemptSourceManual); err != nil {
			rt.logger.Warn("api: git source deploy failed", slog.String("name", name), slog.String("branch", branch), slog.String("error", err.Error()))
		}
	}()
	rt.logger.Info("api: git source deploy started", slog.String("name", name), slog.String("branch", branch))
	writeJSON(w, http.StatusAccepted, gitSourceDeployResponse{App: name, Branch: branch, Started: true})
}
