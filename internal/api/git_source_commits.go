package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envGitCommitsCacheTTL     = "APP_GIT_COMMITS_CACHE_TTL"
	defaultGitCommitsCacheTTL = 30 * time.Second
	gitCommitsLimit           = 10
	maxBranchNameLen          = 255
	maxGitCommitsCacheEntries = 256
	webhookHealthSample       = 10
)

// gitCommitsBundle is the provider-side data one commits request needs,
// cached briefly so reopening the deploy sheet does not hit the provider's
// rate limit. Attempt-derived fields are never cached.
type gitCommitsBundle struct {
	Provider           string
	Commits            []repolayout.Commit
	Branches           []string
	NeedsBuildSettings bool
	LooksLikeMonorepo  bool
	Degraded           bool
	FetchedAt          time.Time
}

type gitCommitsCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]gitCommitsBundle
}

func newGitCommitsCache() *gitCommitsCache {
	ttl := defaultGitCommitsCacheTTL
	if d, err := time.ParseDuration(os.Getenv(envGitCommitsCacheTTL)); err == nil && d >= 0 {
		ttl = d
	}
	return &gitCommitsCache{ttl: ttl, m: map[string]gitCommitsBundle{}}
}

func (c *gitCommitsCache) get(key string) (gitCommitsBundle, bool) {
	if c == nil || c.ttl == 0 {
		return gitCommitsBundle{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	if !ok || time.Since(b.FetchedAt) > c.ttl {
		return gitCommitsBundle{}, false
	}
	return b, true
}

func (c *gitCommitsCache) put(key string, b gitCommitsBundle) {
	if c == nil || c.ttl == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxGitCommitsCacheEntries {
		for k, v := range c.m {
			if time.Since(v.FetchedAt) > c.ttl {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxGitCommitsCacheEntries {
			c.m = map[string]gitCommitsBundle{}
		}
	}
	c.m[key] = b
}

type gitCommitResource struct {
	repolayout.Commit
	// DeployStatus is the newest attempt's outcome for this commit:
	// "deployed", "succeeded", "failed", "running", or empty when never built.
	DeployStatus string `json:"deploy_status,omitempty"`
	DeployID     string `json:"deploy_id,omitempty"`
}

type gitDeployRef struct {
	DeployID   string    `json:"deploy_id"`
	Image      string    `json:"image"`
	CommitSHA  string    `json:"commit_sha,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

type gitSourceCommitsResponse struct {
	RepoURL            string                 `json:"repo_url"`
	Branch             string                 `json:"branch"`
	ConnectedBranch    string                 `json:"connected_branch"`
	Provider           string                 `json:"provider"`
	Head               *gitCommitResource     `json:"head,omitempty"`
	Commits            []gitCommitResource    `json:"commits"`
	Branches           []string               `json:"branches"`
	Deployed           *gitDeployRef          `json:"deployed,omitempty"`
	AheadBy            *int                   `json:"ahead_by,omitempty"`
	UpToDate           bool                   `json:"up_to_date"`
	LastSuccessful     *gitDeployRef          `json:"last_successful,omitempty"`
	PreviousSuccessful *gitDeployRef          `json:"previous_successful,omitempty"`
	Build              gitSourceResolvedBuild `json:"build"`
	BuildType          string                 `json:"build_type"`
	TriggerMode        string                 `json:"trigger_mode"`
	Suggestions        []deploySuggestion     `json:"suggestions"`
	Degraded           bool                   `json:"degraded"`
	FetchedAt          time.Time              `json:"fetched_at"`
}

// validBranchName rejects refs that could be read as options or escape the
// provider URL path: the value only ever reaches a query string or a path
// segment after escaping, so this is a clarity gate, not the only defense.
func validBranchName(b string) bool {
	if b == "" || len(b) > maxBranchNameLen || strings.HasPrefix(b, "-") || strings.Contains(b, "..") {
		return false
	}
	for _, r := range b {
		if r < 0x20 || r == 0x7f || r == ' ' || r == '~' || r == '^' || r == ':' || r == '?' || r == '*' || r == '[' || r == '\\' {
			return false
		}
	}
	return true
}

// handleListGitSourceCommits handles GET /api/v1/apps/{name}/git-source/commits:
// the newest commits of a branch with what is deployed against them, the
// branch list, and computed deploy suggestions, so the deploy sheet can offer
// one-click deploys instead of a form.
func (rt *Router) handleListGitSourceCommits(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: list git source commits: load failed", err, slog.String("name", name))
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = gs.Branch
	}
	if !validBranchName(branch) {
		writeError(w, http.StatusBadRequest, "branch is not a valid branch name")
		return
	}
	if err := requireHTTPOrHTTPSScheme(gs.RepoURL); err != nil {
		writeError(w, http.StatusBadRequest, "repo_url must use http or https")
		return
	}

	key := name + "\x00" + gs.RepoURL + "\x00" + branch
	bundle, hit := rt.gitCommits.get(key)
	if !hit || r.URL.Query().Get("refresh") == "1" {
		token := rt.tokenForRepo(ctx, name, gs.RepoURL, gitHubAppTokenAllowed(ctx))
		bundle, err = rt.loadGitCommitsBundle(ctx, gs, branch, token)
		if err != nil {
			rt.writeCommitsError(w, name, gs.RepoURL, err)
			return
		}
		rt.gitCommits.put(key, bundle)
	}

	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.internalError(w, "api: list git source commits: list attempts failed", err, slog.String("name", name))
		return
	}
	resp := rt.buildGitCommitsResponse(ctx, name, gs, branch, bundle, attempts)
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) loadGitCommitsBundle(ctx context.Context, gs *store.GitSource, branch, token string) (gitCommitsBundle, error) {
	open := rt.repoLayoutSource
	if open == nil {
		open = func(_ context.Context, repoURL, br, tok string, hosts repolayout.Hosts) (repolayout.Source, error) {
			return repolayout.NewAPISource(gitprovider.NewGuardedClient(), repoURL, br, tok, hosts)
		}
	}
	src, err := open(ctx, gs.RepoURL, branch, token, rt.repoLayoutHosts(ctx))
	if errors.Is(err, repolayout.ErrUnsupportedHost) {
		return rt.headOnlyBundle(ctx, gs.RepoURL, branch, token)
	}
	if err != nil {
		return gitCommitsBundle{}, err
	}
	cs, ok := src.(repolayout.CommitSource)
	if !ok {
		return rt.headOnlyBundle(ctx, gs.RepoURL, branch, token)
	}
	commits, err := cs.Commits(ctx, gitCommitsLimit)
	if err != nil {
		return gitCommitsBundle{}, err
	}
	b := gitCommitsBundle{Provider: cs.Provider(), Commits: commits, FetchedAt: time.Now()}
	if branches, berr := cs.Branches(ctx); berr == nil {
		b.Branches = branches
	}
	if res, derr := repolayout.Detect(ctx, src); derr == nil {
		b.LooksLikeMonorepo = res.LooksLikeMonorepo
		b.NeedsBuildSettings = gs.BuildType == "railpack" && gs.BaseDirectory == "" && !res.RootHasApp &&
			(len(res.Dockerfiles) > 0 || res.LooksLikeMonorepo)
	}
	return b, nil
}

// headOnlyBundle serves hosts without a commit-list API: the branch head
// from the advertised refs, so a one-click deploy still works.
func (rt *Router) headOnlyBundle(ctx context.Context, repoURL, branch, token string) (gitCommitsBundle, error) {
	if err := build.ValidatePublicRepoURL(repoURL); err != nil {
		return gitCommitsBundle{}, err
	}
	sha, err := rt.resolveBranchSHA(ctx, repoURL, branch, token)
	if err != nil {
		return gitCommitsBundle{}, err
	}
	return gitCommitsBundle{Provider: "git", Commits: []repolayout.Commit{{SHA: sha}}, Degraded: true, FetchedAt: time.Now()}, nil
}

func (rt *Router) writeCommitsError(w http.ResponseWriter, name, repoURL string, err error) {
	var se *repolayout.StatusError
	switch {
	case errors.As(err, &se) && se.Status == http.StatusNotFound:
		writeError(w, http.StatusNotFound, "the repository or branch was not found, or the stored token cannot see it")
	case errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden):
		writeError(w, http.StatusUnprocessableEntity, "the provider refused access to this repository, check the deploy token, the app installation, or the provider rate limit")
	case errors.As(err, &se) && se.Status == http.StatusTooManyRequests:
		writeError(w, http.StatusTooManyRequests, "the provider is rate limiting requests, try again shortly")
	default:
		rt.logger.Error("api: list git source commits failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", redactURLCredentials(repoURL)))
		writeError(w, http.StatusBadGateway, "could not read commits from the repository provider")
	}
}

func (rt *Router) buildGitCommitsResponse(ctx context.Context, name string, gs *store.GitSource, branch string, b gitCommitsBundle, attempts []store.DeployAttempt) gitSourceCommitsResponse {
	resp := gitSourceCommitsResponse{
		RepoURL: redactURLCredentials(gs.RepoURL), Branch: branch, ConnectedBranch: gs.Branch,
		Provider: b.Provider, Branches: b.Branches, BuildType: gs.BuildType, TriggerMode: gs.TriggerMode,
		Build:    resolveGitSourceBuild(gs.BuildType, gs.BaseDirectory, gs.BuildPath, gs.BuildCommand, gs.PublishDir),
		Degraded: b.Degraded, FetchedAt: b.FetchedAt,
		Commits: make([]gitCommitResource, 0, len(b.Commits)),
	}
	if resp.Branches == nil {
		resp.Branches = []string{}
	}
	if resp.TriggerMode == "" {
		resp.TriggerMode = "push"
	}

	latestBySHA := map[string]store.DeployAttempt{}
	var deployed, last, prev *store.DeployAttempt
	for i := range attempts {
		a := attempts[i]
		if a.CommitSHA != "" {
			if _, ok := latestBySHA[a.CommitSHA]; !ok {
				latestBySHA[a.CommitSHA] = a
			}
		}
		if a.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		if deployed == nil && a.CommitSHA != "" {
			deployed = &attempts[i]
		}
		switch {
		case last == nil:
			last = &attempts[i]
		case prev == nil && a.Image != last.Image:
			prev = &attempts[i]
		}
	}
	if deployed != nil {
		resp.Deployed = deployRef(deployed)
	}
	resp.LastSuccessful = deployRef(last)
	resp.PreviousSuccessful = deployRef(prev)

	deployedIdx := -1
	for i, c := range b.Commits {
		res := gitCommitResource{Commit: c}
		if a, ok := latestBySHA[c.SHA]; ok {
			res.DeployID = a.ID
			res.DeployStatus = a.Status
		}
		if deployed != nil && c.SHA == deployed.CommitSHA {
			res.DeployStatus = "deployed"
			deployedIdx = i
		}
		resp.Commits = append(resp.Commits, res)
	}
	if len(resp.Commits) > 0 {
		resp.Head = &resp.Commits[0]
	}
	if deployedIdx >= 0 {
		resp.AheadBy = &deployedIdx
		resp.UpToDate = deployedIdx == 0
	}

	resp.Suggestions = rt.deploySuggestionsFor(ctx, name, gs, b, resp, attempts)
	return resp
}

func deployRef(a *store.DeployAttempt) *gitDeployRef {
	if a == nil {
		return nil
	}
	ref := &gitDeployRef{DeployID: a.ID, Image: a.Image, CommitSHA: a.CommitSHA, FinishedAt: a.StartedAt}
	if a.FinishedAt != nil {
		ref.FinishedAt = *a.FinishedAt
	}
	return ref
}

func (rt *Router) deploySuggestionsFor(ctx context.Context, name string, gs *store.GitSource, b gitCommitsBundle, resp gitSourceCommitsResponse, attempts []store.DeployAttempt) []deploySuggestion {
	in := suggestionInput{
		Provider: b.Provider, TriggerMode: resp.TriggerMode, PreviewEnabled: gs.PreviewEnabled,
		NeedsBuildSettings: b.NeedsBuildSettings, LooksLikeMonorepo: b.LooksLikeMonorepo, BaseDirectory: gs.BaseDirectory,
	}
	if resp.Head != nil {
		in.HeadSHA = resp.Head.SHA
		if resp.Head.DeployStatus == store.DeployAttemptStatusFailed {
			in.HeadFailedDeployID = resp.Head.DeployID
		}
	}
	if len(attempts) > 0 && attempts[0].Status == store.DeployAttemptStatusFailed {
		if f, ok := failure.Classify(attemptFailureInput(attempts[0], ""), failureOptions()); ok {
			in.LastFailureCode = f.Code
		}
	}
	if rt.webhookDeliveries != nil {
		if ds, err := rt.webhookDeliveries.ListWebhookDeliveries(ctx, name, webhookHealthSample, nil); err == nil {
			for i, d := range ds {
				if d.SignatureValid && d.Matched {
					in.WebhookSeen = true
				}
				if i == 0 && !d.SignatureValid {
					in.WebhookBadSignature = true
				}
			}
		}
	}
	if svc, err := rt.apps.GetDesiredService(ctx, name); err == nil && svc != nil && svc.Health != nil {
		in.HasHealthCheck = svc.Health.Readiness != nil || svc.Health.Liveness != nil
	}
	if rt.deployNotifyTargets != nil {
		if ts, err := rt.deployNotifyTargets.ListDeployTargetsForResource(ctx, resourceIDForApp(name)); err == nil {
			in.HasNotifyTarget = len(ts) > 0
		}
	}
	if rt.notificationChannels != nil {
		if cs, err := rt.notificationChannels.ListNotificationChannels(ctx); err == nil {
			in.ChannelsExist = len(cs) > 0
		}
	}
	out := computeDeploySuggestions(in)
	if out == nil {
		out = []deploySuggestion{}
	}
	return out
}
