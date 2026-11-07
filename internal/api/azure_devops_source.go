package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/azuredevops"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolist"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// AzureDevOpsClient is the surface internal/api needs from internal/azuredevops.Client.
type AzureDevOpsClient interface {
	CurrentUser(ctx context.Context, instanceURL, token string) (azuredevops.Account, error)
	ListRepos(ctx context.Context, instanceURL, token string, opts gitprovider.ListOptions) ([]azuredevops.Repo, gitprovider.ListResult, error)
	GetRepo(ctx context.Context, instanceURL, token, repo string) (azuredevops.Repo, error)
	ListBranches(ctx context.Context, instanceURL, token, repo string) ([]azuredevops.Branch, error)
	CreateRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL, secret string) error
	DeleteRepoWebhook(ctx context.Context, instanceURL, token, repo, hookURL string) error
}

// azureDevOpsAdapter plugs Azure DevOps into the shared git connection flow: a personal
// access token, no OAuth application, repositories addressed as PROJECT/repo.
func (rt *Router) azureDevOpsAdapter() gitProviderAdapter {
	c := rt.azureDevOpsClient
	return gitProviderAdapter{
		provider: gitProviderAzureDevOps, label: "Azure DevOps", patAuth: true,
		revokeNote: "Revoke the personal access token under User settings, Personal access tokens in Azure DevOps.",
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

func (rt *Router) azureDevOpsRepos(ctx context.Context, conn store.GitConnection, token string) ([]repolist.Repo, gitprovider.ListResult, error) {
	list, res, err := rt.azureDevOpsClient.ListRepos(ctx, conn.InstanceURL, token, rt.repoListOptions())
	repos := make([]repolist.Repo, 0, len(list))
	for _, r := range list {
		repos = append(repos, repolist.Repo{
			Key: r.FullName, FullName: r.FullName, Name: r.Name, Owner: ownerOf(r.FullName), Private: r.Private,
			DefaultBranch: r.DefaultBranch, CloneURL: r.CloneURL, WebURL: r.WebURL,
		})
	}
	return repos, res, err
}

// parseProviderPushEvent picks the push parser by delivery shape: Azure DevOps hooks carry
// no event header, only an eventType in the body.
func parseProviderPushEvent(body []byte, header http.Header) (webhook.PushEvent, error) {
	if azuredevops.IsPushPayload(body) {
		return azuredevops.ParsePushEvent(body)
	}
	return webhook.ParsePushEventForProvider(body, header.Get(webhook.HeaderBitbucketEventKey))
}
