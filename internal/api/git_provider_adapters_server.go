package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/bitbucketserver"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolist"
	"github.com/GLINCKER/levelrail/internal/store"
)

const forgejoCallbackPath = "/api/v1/forgejo-app/callback"

// bitbucketServerTokenExpiryYears is the stored expiry of a personal access token, so the
// shared token path never tries to refresh it; the server revokes it, not a clock.
const bitbucketServerTokenExpiryYears = 100

// BitbucketServerClient is the surface internal/api needs from internal/bitbucketserver.Client.
type BitbucketServerClient interface {
	CurrentUser(ctx context.Context, instanceURL, token string) (bitbucketserver.Account, error)
	ListRepos(ctx context.Context, instanceURL, token string, opts gitprovider.ListOptions) ([]bitbucketserver.Repo, gitprovider.ListResult, error)
	GetRepo(ctx context.Context, instanceURL, token, repo string) (bitbucketserver.Repo, error)
	DefaultBranch(ctx context.Context, instanceURL, token, repo string) (string, error)
	ListBranches(ctx context.Context, instanceURL, token, repo string) ([]bitbucketserver.Branch, error)
	CreateRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL, secret string) error
	DeleteRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL string) error
}

// forgejoAdapter is the Gitea adapter under Forgejo's own provider kind and label: Forgejo keeps Gitea's OAuth2 and REST API.
func (rt *Router) forgejoAdapter() gitProviderAdapter {
	a := rt.giteaAdapter()
	a.provider, a.label, a.callbackPath = gitProviderForgejo, "Forgejo", forgejoCallbackPath
	a.scopes = giteaapp.DefaultScope
	a.revokeNote = "Forgejo has no API to revoke an OAuth2 application's tokens. Remove the application under Settings, Applications on your Forgejo instance."
	return a
}

func (rt *Router) bitbucketServerAdapter() gitProviderAdapter {
	c := rt.bitbucketServerClient
	return gitProviderAdapter{
		provider: gitProviderBitbucketServer, label: "Bitbucket Server", patAuth: true,
		revokeNote: "Delete the personal access token under Manage account, Personal access tokens on your Bitbucket Server instance.",
		account: func(ctx context.Context, conn store.GitConnection, token string) (gitAccountInfo, error) {
			a, err := c.CurrentUser(ctx, conn.InstanceURL, token)
			return gitAccountInfo{ID: a.ID, Login: a.Login}, err
		},
		lookup: func(ctx context.Context, conn store.GitConnection, token, repo string) (gitRepoInfo, error) {
			r, err := c.GetRepo(ctx, conn.InstanceURL, token, repo)
			return gitRepoInfo{CloneURL: r.CloneURL, DefaultBranch: r.DefaultBranch}, err
		},
		branches: func(ctx context.Context, conn store.GitConnection, token, repo string) ([]gitBranchInfo, error) {
			bs, err := c.ListBranches(ctx, conn.InstanceURL, token, repo)
			out := make([]gitBranchInfo, 0, len(bs))
			for _, b := range bs {
				out = append(out, gitBranchInfo{Name: b.Name, CommitSHA: b.CommitSHA})
			}
			return out, err
		},
		webhook: func(ctx context.Context, conn store.GitConnection, token, repo, hookURL, secret string) error {
			return c.CreateRepoWebhook(ctx, conn.InstanceURL, token, repo, hookURL, secret)
		},
		deleteWebhook: func(ctx context.Context, conn store.GitConnection, token, repo, hookURL string) error {
			return c.DeleteRepoWebhook(ctx, conn.InstanceURL, token, repo, hookURL)
		},
	}
}

func bitbucketServerRepoToRepo(r bitbucketserver.Repo) repolist.Repo {
	return repolist.Repo{
		Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: ownerOf(r.FullName), Private: r.Private,
		DefaultBranch: r.DefaultBranch, CloneURL: r.CloneURL, WebURL: r.WebURL,
	}
}
