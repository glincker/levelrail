package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolist"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Provider names accepted by the repository list endpoints, and the cache
// key prefixes their connections live under.
const (
	gitProviderGitHub    = "github"
	gitProviderGitLab    = "gitlab"
	gitProviderBitbucket = "bitbucket"
	gitProviderGitea     = "gitea"
)

// repoListConnError marks a failure to reach the connection itself (not
// connected, not authorized, no key), as opposed to a provider listing failure.
type repoListConnError struct{ err error }

func (e *repoListConnError) Error() string { return e.err.Error() }
func (e *repoListConnError) Unwrap() error { return e.err }

// repoListSource is one cache key's worth of repositories: a whole
// connection, or one GitHub installation.
type repoListSource struct {
	key         string
	account     string
	accountType string
	fetch       repolist.FetchFunc
}

// repoListAccount is one source's outcome.
type repoListAccount struct {
	Account string
	Result  repolist.Result
	Err     error
}

func (rt *Router) invalidateGitRepoCache(ctx context.Context, prefix string) {
	if rt.repoCache == nil {
		return
	}
	if err := rt.repoCache.Invalidate(ctx, prefix); err != nil {
		rt.logger.Warn("api: invalidate git repo list cache failed", "cache_prefix", prefix, "error", err.Error())
	}
}

func (rt *Router) gitRepoSources(ctx context.Context, provider string) ([]repoListSource, error) {
	switch provider {
	case gitProviderGitHub:
		return rt.githubRepoSources(ctx)
	case gitProviderGitLab:
		if _, err := rt.gitlabApp.GetGitLabAppConnection(ctx); err != nil {
			if errors.Is(err, store.ErrGitLabAppConnectionNotFound) {
				return nil, &repoListConnError{errGitLabAppNotConnected}
			}
			return nil, fmt.Errorf("get gitlab app connection: %w", err)
		}
		return []repoListSource{{key: gitProviderGitLab, fetch: rt.gitlabRepoFetch}}, nil
	case gitProviderBitbucket:
		if _, err := rt.bitbucketApp.GetBitbucketAppConnection(ctx); err != nil {
			if errors.Is(err, store.ErrBitbucketAppConnectionNotFound) {
				return nil, &repoListConnError{errBitbucketAppNotConnected}
			}
			return nil, fmt.Errorf("get bitbucket app connection: %w", err)
		}
		return []repoListSource{{key: gitProviderBitbucket, fetch: rt.bitbucketRepoFetch}}, nil
	case gitProviderGitea:
		if _, err := rt.giteaApp.GetGiteaAppConnection(ctx); err != nil {
			if errors.Is(err, store.ErrGiteaAppConnectionNotFound) {
				return nil, &repoListConnError{errGiteaAppNotConnected}
			}
			return nil, fmt.Errorf("get gitea app connection: %w", err)
		}
		return []repoListSource{{key: gitProviderGitea, fetch: rt.giteaRepoFetch}}, nil
	}
	return nil, fmt.Errorf("unknown git provider %q", provider)
}

func (rt *Router) githubRepoSources(ctx context.Context) ([]repoListSource, error) {
	installations, err := rt.githubApp.ListGitHubAppInstallations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list github app installations: %w", err)
	}
	if len(installations) == 0 {
		return []repoListSource{{
			key: gitProviderGitHub + ":legacy", accountType: "organization",
			fetch: func(ctx context.Context, prior *repolist.Snapshot) (repolist.Snapshot, error) {
				instanceURL, token, tokErr := rt.mintGitHubAppInstallationToken(ctx)
				if tokErr != nil {
					return repolist.Snapshot{}, &repoListConnError{tokErr}
				}
				return rt.githubListSnapshot(ctx, instanceURL, token, "organization", prior)
			},
		}}, nil
	}
	out := make([]repoListSource, 0, len(installations))
	for _, inst := range installations {
		out = append(out, repoListSource{
			key:         gitProviderGitHub + ":" + strconv.FormatInt(inst.InstallationID, 10),
			account:     inst.AccountLogin,
			accountType: inst.AccountType,
			fetch: func(ctx context.Context, prior *repolist.Snapshot) (repolist.Snapshot, error) {
				instanceURL, token, tokErr := rt.mintGitHubAppInstallationTokenForOwner(ctx, inst.AccountLogin)
				if tokErr != nil {
					return repolist.Snapshot{}, &repoListConnError{tokErr}
				}
				return rt.githubListSnapshot(ctx, instanceURL, token, inst.AccountType, prior)
			},
		})
	}
	return out, nil
}

func (rt *Router) repoListOptions() gitprovider.ListOptions {
	return gitprovider.ListOptions{MaxItems: rt.repoCache.Config().MaxItems, Retry: gitprovider.RetryPolicyFromEnv()}
}

func (rt *Router) githubListSnapshot(ctx context.Context, instanceURL, token, accountType string, prior *repolist.Snapshot) (repolist.Snapshot, error) {
	opts := githubapp.ListOptions{ListOptions: rt.repoListOptions()}
	if prior != nil && !prior.Truncated {
		var etags []string
		if json.Unmarshal([]byte(prior.Meta), &etags) == nil {
			opts.PriorETags = etags
			opts.PriorRepos = githubReposFromNeutral(prior.Repos)
		}
	}
	repos, res, err := rt.githubAppClient.ListInstallationRepos(ctx, instanceURL, token, opts)
	if err != nil {
		return repolist.Snapshot{}, fmt.Errorf("list github installation repos: %w", err)
	}
	meta, err := json.Marshal(res.ETags)
	if err != nil {
		return repolist.Snapshot{}, fmt.Errorf("encode github page etags: %w", err)
	}
	out := make([]repolist.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, repolist.Repo{
			Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: r.OwnerLogin, Private: r.Private,
			Archived: r.Archived, Fork: r.Fork, DefaultBranch: r.DefaultBranch,
			CloneURL: instanceURL + "/" + r.FullName + ".git", WebURL: r.HTMLURL,
			Language: r.Language, PushedAt: r.PushedAt, AccountType: accountType,
		})
	}
	return repolist.Snapshot{Repos: out, Truncated: res.Truncated, Meta: string(meta)}, nil
}

func githubReposFromNeutral(repos []repolist.Repo) []githubapp.Repo {
	out := make([]githubapp.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, githubapp.Repo{
			FullName: r.FullName, Name: r.Name, OwnerLogin: r.Owner, Private: r.Private,
			DefaultBranch: r.DefaultBranch, HTMLURL: r.WebURL, Language: r.Language, PushedAt: r.PushedAt,
			Archived: r.Archived, Fork: r.Fork,
		})
	}
	return out
}

func (rt *Router) gitlabRepoFetch(ctx context.Context, _ *repolist.Snapshot) (repolist.Snapshot, error) {
	conn, accessToken, err := rt.gitlabAccessToken(ctx)
	if err != nil {
		return repolist.Snapshot{}, &repoListConnError{err}
	}
	projects, res, err := rt.gitlabAppClient.ListProjects(ctx, conn.InstanceURL, accessToken, rt.repoListOptions())
	if err != nil {
		return repolist.Snapshot{}, fmt.Errorf("list gitlab projects: %w", err)
	}
	out := make([]repolist.Repo, 0, len(projects))
	for _, p := range projects {
		out = append(out, gitlabProjectToRepo(p))
	}
	return repolist.Snapshot{Repos: out, Truncated: res.Truncated}, nil
}

func gitlabProjectToRepo(p gitlabapp.Project) repolist.Repo {
	return repolist.Repo{
		Key: strconv.FormatInt(p.ID, 10), FullName: p.PathWithNamespace, Name: p.Name,
		Owner: ownerOf(p.PathWithNamespace), Private: p.Visibility != "public", Archived: p.Archived, Fork: p.Fork,
		DefaultBranch: p.DefaultBranch, CloneURL: p.HTTPURLToRepo, WebURL: p.WebURL, ProjectID: p.ID,
	}
}

func (rt *Router) bitbucketRepoFetch(ctx context.Context, _ *repolist.Snapshot) (repolist.Snapshot, error) {
	accessToken, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		return repolist.Snapshot{}, &repoListConnError{err}
	}
	repos, res, err := rt.bitbucketAppClient.ListRepos(ctx, accessToken, rt.repoListOptions())
	if err != nil {
		return repolist.Snapshot{}, fmt.Errorf("list bitbucket repos: %w", err)
	}
	out := make([]repolist.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, bitbucketRepoToRepo(r))
	}
	return repolist.Snapshot{Repos: out, Truncated: res.Truncated}, nil
}

func bitbucketRepoToRepo(r bitbucketapp.Repo) repolist.Repo {
	return repolist.Repo{
		Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: ownerOf(r.FullName), Private: r.Private,
		Fork: r.Fork, DefaultBranch: r.DefaultBranch, CloneURL: r.CloneURL, WebURL: r.WebURL,
	}
}

func (rt *Router) giteaRepoFetch(ctx context.Context, _ *repolist.Snapshot) (repolist.Snapshot, error) {
	conn, accessToken, err := rt.giteaAccessToken(ctx)
	if err != nil {
		return repolist.Snapshot{}, &repoListConnError{err}
	}
	repos, res, err := rt.giteaAppClient.ListRepos(ctx, conn.InstanceURL, accessToken, rt.repoListOptions())
	if err != nil {
		return repolist.Snapshot{}, fmt.Errorf("list gitea repos: %w", err)
	}
	out := make([]repolist.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, giteaRepoToRepo(r))
	}
	return repolist.Snapshot{Repos: out, Truncated: res.Truncated}, nil
}

func giteaRepoToRepo(r giteaapp.Repo) repolist.Repo {
	return repolist.Repo{
		Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: ownerOf(r.FullName), Private: r.Private,
		Archived: r.Archived, Fork: r.Fork, DefaultBranch: r.DefaultBranch, CloneURL: r.CloneURL, WebURL: r.WebURL,
	}
}

// ownerOf is the namespace part of "owner/name" or "group/sub/name".
func ownerOf(fullName string) string {
	i := strings.LastIndex(fullName, "/")
	if i < 0 {
		return ""
	}
	return fullName[:i]
}

// loadGitRepos returns every source's cached-or-fetched list. The error is
// connection-level: nothing could be listed because no usable connection exists.
func (rt *Router) loadGitRepos(ctx context.Context, provider string, refresh bool) ([]repoListAccount, error) {
	sources, err := rt.gitRepoSources(ctx, provider)
	if err != nil {
		return nil, err
	}
	out := make([]repoListAccount, len(sources))
	done := make(chan struct{}, len(sources))
	for i, src := range sources {
		go func() {
			defer func() { done <- struct{}{} }()
			res, getErr := rt.repoCache.Get(ctx, src.key, provider, refresh, src.fetch)
			out[i] = repoListAccount{Account: src.account, Result: res, Err: getErr}
		}()
	}
	for range sources {
		<-done
	}
	var connErr *repoListConnError
	failed := 0
	for _, a := range out {
		if a.Err != nil {
			failed++
			if errors.As(a.Err, &connErr) && failed == len(out) {
				return nil, connErr
			}
		}
	}
	return out, nil
}

// attachRepoCache builds the repo list cache over the main store when it
// can persist one, and over memory otherwise.
func (rt *Router) attachRepoCache(s Store) {
	var st repolist.Store
	if persist, ok := any(s).(repolist.Store); ok {
		st = persist
	} else {
		st = repolist.NewMemoryStore()
	}
	rt.repoCache = repolist.NewCache(st, repolist.ConfigFromEnv(), rt.logger)
}
