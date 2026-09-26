package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/pipeline"
)

// EnvGitStatusEnabled is the server kill switch for posting commit statuses
// and deployments back to git forges. Unset means on.
const EnvGitStatusEnabled = "APP_GIT_STATUS_ENABLED"

// Forge provider names recorded on runs and deployments.
const (
	forgeGitHub    = "github"
	forgeGitLab    = "gitlab"
	forgeGitea     = "gitea"
	forgeBitbucket = "bitbucket"
)

const bitbucketKeyMax = 40

// errNoForge means the repository is not on a forge this control plane holds
// credentials for; callers treat it as "nothing to report to".
var errNoForge = errors.New("api: no usable git forge connection for this repository")

func gitStatusEnabled() bool {
	v, err := strconv.ParseBool(os.Getenv(EnvGitStatusEnabled))
	return err != nil || v
}

type githubForgeClient interface {
	CreateCommitStatus(ctx context.Context, instanceURL, token, owner, repo, sha string, state githubapp.CommitStatusState, targetURL, description, statusContext string) error
	CreateDeployment(ctx context.Context, instanceURL, token, owner, repo, ref, environment, description string, production bool) (int64, error)
	CreateDeploymentStatus(ctx context.Context, instanceURL, token, owner, repo string, id int64, state githubapp.DeploymentState, environmentURL, logURL, description string) error
	CompareChangedFiles(ctx context.Context, instanceURL, token, owner, repo, base, head string) ([]string, error)
	PullRequestFiles(ctx context.Context, instanceURL, token, owner, repo string, number int) ([]string, error)
}

type gitlabForgeClient interface {
	CreateCommitStatus(ctx context.Context, instanceURL, accessToken, projectPath, sha string, state gitlabapp.CommitState, targetURL, description, name string) error
	CompareChangedFiles(ctx context.Context, instanceURL, accessToken, projectPath, from, to string) ([]string, error)
	MergeRequestChangedFiles(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int) ([]string, error)
}

type giteaForgeClient interface {
	CreateCommitStatus(ctx context.Context, instanceURL, accessToken, fullName, sha string, state giteaapp.CommitStatusState, targetURL, description, statusContext string) error
	CompareChangedFiles(ctx context.Context, instanceURL, accessToken, fullName, base, head string) ([]string, error)
	PullRequestChangedFiles(ctx context.Context, instanceURL, accessToken, fullName string, number int) ([]string, error)
}

type bitbucketForgeClient interface {
	CreateCommitBuildStatus(ctx context.Context, accessToken, fullName, commit string, state bitbucketapp.BuildStatusState, targetURL, description, key string) error
	CompareChangedFiles(ctx context.Context, accessToken, fullName, head, base string) ([]string, error)
	PullRequestChangedFiles(ctx context.Context, accessToken, fullName string, prID int) ([]string, error)
}

// forge is one repository on one connected git provider, with the
// credentials to talk to it.
type forge struct {
	kind        string
	instanceURL string
	token       string
	// repo is "owner/name" (GitHub, Gitea, Bitbucket) or the namespaced
	// project path (GitLab).
	repo string

	github    githubForgeClient
	gitlab    gitlabForgeClient
	gitea     giteaForgeClient
	bitbucket bitbucketForgeClient
}

// resolveForge finds the connected provider that hosts repoURL. It returns
// errNoForge when none does.
func (rt *Router) resolveForge(ctx context.Context, repoURL string) (*forge, error) {
	if repoURL == "" {
		return nil, errNoForge
	}
	if c, ok := rt.githubAppClient.(githubForgeClient); ok && rt.githubApp != nil && rt.githubAppSecrets != nil {
		if instanceURL, token, err := rt.mintGitHubAppInstallationToken(ctx); err == nil {
			if owner, name, ok := githubOwnerRepoFromURL(repoURL, instanceURL); ok {
				return &forge{kind: forgeGitHub, instanceURL: instanceURL, token: token, repo: owner + "/" + name, github: c}, nil
			}
		}
	}
	if c, ok := rt.gitlabAppClient.(gitlabForgeClient); ok && rt.gitlabAppSecrets != nil {
		if conn, token, err := rt.gitlabAccessToken(ctx); err == nil {
			if path, ok := gitlabProjectPathFromURL(repoURL, conn.InstanceURL); ok {
				return &forge{kind: forgeGitLab, instanceURL: conn.InstanceURL, token: token, repo: path, gitlab: c}, nil
			}
		}
	}
	if c, ok := rt.giteaAppClient.(giteaForgeClient); ok && rt.giteaAppSecrets != nil {
		if conn, token, err := rt.giteaAccessToken(ctx); err == nil {
			if full, ok := giteaFullNameFromURL(repoURL, conn.InstanceURL); ok {
				return &forge{kind: forgeGitea, instanceURL: conn.InstanceURL, token: token, repo: full, gitea: c}, nil
			}
		}
	}
	if c, ok := rt.bitbucketAppClient.(bitbucketForgeClient); ok && rt.bitbucketAppSecrets != nil {
		if full, ok := bitbucketFullNameFromURL(repoURL); ok {
			if token, err := rt.bitbucketAccessToken(ctx); err == nil {
				return &forge{kind: forgeBitbucket, token: token, repo: full, bitbucket: c}, nil
			}
		}
	}
	return nil, errNoForge
}

func (f *forge) ownerName() (owner, name string) {
	owner, name, _ = strings.Cut(f.repo, "/")
	return owner, name
}

// postStatus sets a commit status. state is a pipeline.Report* value.
func (f *forge) postStatus(ctx context.Context, sha, state, targetURL, description, name string) error {
	switch f.kind {
	case forgeGitHub:
		owner, repo := f.ownerName()
		return f.github.CreateCommitStatus(ctx, f.instanceURL, f.token, owner, repo, sha,
			githubStatusState(state), targetURL, truncateStatusDescription(description, githubStatusDescriptionMax), name)
	case forgeGitLab:
		return f.gitlab.CreateCommitStatus(ctx, f.instanceURL, f.token, f.repo, sha,
			gitlabStatusState(state), targetURL, truncateStatusDescription(description, gitLabStatusDescriptionMax), name)
	case forgeGitea:
		return f.gitea.CreateCommitStatus(ctx, f.instanceURL, f.token, f.repo, sha,
			giteaStatusState(state), targetURL, description, name)
	case forgeBitbucket:
		key := name
		if len(key) > bitbucketKeyMax {
			key = key[:bitbucketKeyMax]
		}
		return f.bitbucket.CreateCommitBuildStatus(ctx, f.token, f.repo, sha, bitbucketStatusState(state), targetURL, description, key)
	}
	return errNoForge
}

func githubStatusState(state string) githubapp.CommitStatusState {
	switch state {
	case pipeline.ReportSuccess:
		return githubapp.CommitStatusSuccess
	case pipeline.ReportFailure:
		return githubapp.CommitStatusFailure
	case pipeline.ReportError:
		return githubapp.CommitStatusError
	}
	return githubapp.CommitStatusPending
}

func gitlabStatusState(state string) gitlabapp.CommitState {
	switch state {
	case pipeline.ReportSuccess:
		return gitlabapp.CommitStateSuccess
	case pipeline.ReportFailure:
		return gitlabapp.CommitStateFailed
	case pipeline.ReportError:
		return gitlabapp.CommitStateCanceled
	}
	return gitlabapp.CommitStateRunning
}

func giteaStatusState(state string) giteaapp.CommitStatusState {
	switch state {
	case pipeline.ReportSuccess:
		return giteaapp.CommitStatusSuccess
	case pipeline.ReportFailure:
		return giteaapp.CommitStatusFailure
	case pipeline.ReportError:
		return giteaapp.CommitStatusError
	}
	return giteaapp.CommitStatusPending
}

func bitbucketStatusState(state string) bitbucketapp.BuildStatusState {
	switch state {
	case pipeline.ReportSuccess:
		return bitbucketapp.BuildStatusSuccessful
	case pipeline.ReportFailure:
		return bitbucketapp.BuildStatusFailed
	case pipeline.ReportError:
		return bitbucketapp.BuildStatusStopped
	}
	return bitbucketapp.BuildStatusInProgress
}

// changeQuery names what to diff: a pull request, or the range Base..Head.
type changeQuery struct {
	PR   int
	Base string
	Head string
}

var errNoChangeBase = errors.New("api: no base commit to compare against")

func isZeroSHA(s string) bool { return strings.Trim(s, "0") == "" }

// changedFiles lists the files a pull request or commit range touched.
func (f *forge) changedFiles(ctx context.Context, q changeQuery) ([]string, error) {
	if q.PR == 0 && (isZeroSHA(q.Base) || q.Head == "") {
		return nil, errNoChangeBase
	}
	switch f.kind {
	case forgeGitHub:
		owner, repo := f.ownerName()
		if q.PR > 0 {
			return f.github.PullRequestFiles(ctx, f.instanceURL, f.token, owner, repo, q.PR)
		}
		return f.github.CompareChangedFiles(ctx, f.instanceURL, f.token, owner, repo, q.Base, q.Head)
	case forgeGitLab:
		if q.PR > 0 {
			return f.gitlab.MergeRequestChangedFiles(ctx, f.instanceURL, f.token, f.repo, q.PR)
		}
		return f.gitlab.CompareChangedFiles(ctx, f.instanceURL, f.token, f.repo, q.Base, q.Head)
	case forgeGitea:
		if q.PR > 0 {
			return f.gitea.PullRequestChangedFiles(ctx, f.instanceURL, f.token, f.repo, q.PR)
		}
		return f.gitea.CompareChangedFiles(ctx, f.instanceURL, f.token, f.repo, q.Base, q.Head)
	case forgeBitbucket:
		if q.PR > 0 {
			return f.bitbucket.PullRequestChangedFiles(ctx, f.token, f.repo, q.PR)
		}
		return f.bitbucket.CompareChangedFiles(ctx, f.token, f.repo, q.Head, q.Base)
	}
	return nil, errNoForge
}

// describeForgeError turns a forge API error into a one-line warning,
// naming the retry hint when the forge rate limited the call.
func describeForgeError(err error) string {
	if after, limited := gitprovider.IsRateLimited(err); limited {
		if after != "" {
			return fmt.Sprintf("rate limited by the git provider (retry after %ss)", after)
		}
		return "rate limited by the git provider"
	}
	return err.Error()
}

// changedFilesFn returns a lazy loader for a pipeline.Event, resolving the
// forge for app's connected repository only when a path filter needs it.
func (rt *Router) changedFilesFn(app string, q changeQuery) func(ctx context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		gs, err := rt.gitSources.GetGitSource(ctx, app)
		if err != nil {
			return nil, fmt.Errorf("load git source: %w", err)
		}
		f, err := rt.resolveForge(ctx, gs.RepoURL)
		if err != nil {
			return nil, err
		}
		files, err := f.changedFiles(ctx, q)
		if err != nil {
			rt.logger.Warn("api: fetch changed files failed", slog.String("app_name", app), slog.String("provider", f.kind), slog.String("error", describeForgeError(err)))
			return nil, fmt.Errorf("%s changed files: %s", f.kind, describeForgeError(err))
		}
		return files, nil
	}
}
