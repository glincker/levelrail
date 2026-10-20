package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/bitbucketserver"
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
	gitProviderGitLab    = store.GitProviderGitLab
	gitProviderBitbucket = store.GitProviderBitbucket
	gitProviderGitea     = store.GitProviderGitea

	gitProviderForgejo         = store.GitProviderForgejo
	gitProviderBitbucketServer = store.GitProviderBitbucketServer
)

// connectionGitProviders are the providers that hold operator-managed connections, in display order.
var connectionGitProviders = []string{gitProviderGitLab, gitProviderBitbucket, gitProviderBitbucketServer, gitProviderGitea, gitProviderForgejo}

// repoListConnError marks a failure to reach the connection itself (not
// connected, not authorized, no key), as opposed to a provider listing failure.
type repoListConnError struct{ err error }

func (e *repoListConnError) Error() string { return e.err.Error() }
func (e *repoListConnError) Unwrap() error { return e.err }

// repoListSource is one cache key's worth of repositories: a whole
// connection, or one GitHub installation.
type repoListSource struct {
	key             string
	account         string
	accountType     string
	connectionID    string
	connectionLabel string
	fetch           repolist.FetchFunc
}

// repoListAccount is one source's outcome.
type repoListAccount struct {
	Provider        string
	ConnectionID    string
	ConnectionLabel string
	Account         string
	Result          repolist.Result
	Err             error
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
	if provider == gitProviderGitHub {
		return rt.githubRepoSources(ctx)
	}
	if !validGitProvider(provider) {
		return nil, fmt.Errorf("unknown git provider %q", provider)
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("list %s connections: %w", provider, err)
	}
	if len(conns) == 0 {
		return nil, &repoListConnError{errGitConnNone}
	}
	out := make([]repoListSource, 0, len(conns))
	for _, c := range conns {
		out = append(out, repoListSource{
			key: gitRepoCacheKey(provider, c.ID), account: c.AccountName, connectionID: c.ID, connectionLabel: c.Label,
			fetch: rt.connectionRepoFetch(c),
		})
	}
	return out, nil
}

func (rt *Router) githubRepoSources(ctx context.Context) ([]repoListSource, error) {
	installations, err := rt.githubApp.ListGitHubAppInstallations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list github app installations: %w", err)
	}
	if len(installations) == 0 {
		return []repoListSource{{
			key: gitProviderGitHub + ":legacy", accountType: "organization", connectionID: gitProviderGitHub + ":legacy", connectionLabel: "GitHub",
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
		id := strconv.FormatInt(inst.InstallationID, 10)
		out = append(out, repoListSource{
			key:             gitProviderGitHub + ":" + id,
			account:         inst.AccountLogin,
			accountType:     inst.AccountType,
			connectionID:    gitProviderGitHub + ":" + id,
			connectionLabel: inst.AccountLogin,
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

// connectionRepoFetch lists one connection's repositories with that
// connection's own token, recording its health under its own id.
func (rt *Router) connectionRepoFetch(c store.GitConnection) repolist.FetchFunc {
	return func(ctx context.Context, _ *repolist.Snapshot) (repolist.Snapshot, error) {
		conn, token, err := rt.gitConnToken(ctx, c.ID)
		if err != nil {
			return repolist.Snapshot{}, &repoListConnError{err}
		}
		var repos []repolist.Repo
		var res gitprovider.ListResult
		switch conn.Provider {
		case gitProviderGitLab:
			var projects []gitlabapp.Project
			projects, res, err = rt.gitlabAppClient.ListProjects(ctx, conn.InstanceURL, token, rt.repoListOptions())
			for _, p := range projects {
				repos = append(repos, gitlabProjectToRepo(p))
			}
		case gitProviderBitbucket:
			var list []bitbucketapp.Repo
			list, res, err = rt.bitbucketAppClient.ListRepos(ctx, token, rt.repoListOptions())
			for _, r := range list {
				repos = append(repos, bitbucketRepoToRepo(r))
			}
		case gitProviderBitbucketServer:
			var list []bitbucketserver.Repo
			list, res, err = rt.bitbucketServerClient.ListRepos(ctx, conn.InstanceURL, token, rt.repoListOptions())
			for _, r := range list {
				repos = append(repos, bitbucketServerRepoToRepo(r))
			}
		default:
			var list []giteaapp.Repo
			list, res, err = rt.giteaAppClient.ListRepos(ctx, conn.InstanceURL, token, rt.repoListOptions())
			for _, r := range list {
				repos = append(repos, giteaRepoToRepo(r))
			}
		}
		if err != nil {
			return repolist.Snapshot{}, rt.gitConnRecordFailure(ctx, conn.Provider, conn.ID, fmt.Errorf("list %s repos: %w", conn.Provider, err), false)
		}
		rt.markGitConnOK(ctx, conn.ID)
		return repolist.Snapshot{Repos: repos, Truncated: res.Truncated}, nil
	}
}

// loadConnectionRepos returns one connection's cached-or-fetched list.
func (rt *Router) loadConnectionRepos(ctx context.Context, c store.GitConnection, refresh bool) repoListAccount {
	res, err := rt.repoCache.Get(ctx, gitRepoCacheKey(c.Provider, c.ID), c.Provider, refresh, rt.connectionRepoFetch(c))
	return repoListAccount{Provider: c.Provider, ConnectionID: c.ID, ConnectionLabel: c.Label, Account: c.AccountName, Result: res, Err: err}
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
		return repolist.Snapshot{}, rt.gitConnRecordFailure(ctx, gitProviderGitHub, gitProviderGitHub, fmt.Errorf("list github installation repos: %w", err), false)
	}
	rt.markGitConnOK(ctx, gitProviderGitHub)
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

func gitlabProjectToRepo(p gitlabapp.Project) repolist.Repo {
	return repolist.Repo{
		Key: strconv.FormatInt(p.ID, 10), FullName: p.PathWithNamespace, Name: p.Name,
		Owner: ownerOf(p.PathWithNamespace), Private: p.Visibility != "public", Archived: p.Archived, Fork: p.Fork,
		DefaultBranch: p.DefaultBranch, CloneURL: p.HTTPURLToRepo, WebURL: p.WebURL, ProjectID: p.ID,
	}
}

func bitbucketRepoToRepo(r bitbucketapp.Repo) repolist.Repo {
	return repolist.Repo{
		Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: ownerOf(r.FullName), Private: r.Private,
		Fork: r.Fork, DefaultBranch: r.DefaultBranch, CloneURL: r.CloneURL, WebURL: r.WebURL,
	}
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
			out[i] = repoListAccount{Provider: provider, ConnectionID: src.connectionID, ConnectionLabel: src.connectionLabel, Account: src.account, Result: res, Err: getErr}
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
