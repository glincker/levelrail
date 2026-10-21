package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	errInternal         = "internal error"
	errGitProviderOneOf = "provider must be one of gitlab, bitbucket, bitbucket_server, gitea, forgejo"
)

// GitConnSecrets is the internal/secrets.Manager surface the git provider
// connection handlers need: the OAuth application's client secret and each
// connection's tokens, under the connection's own secrets key.
type GitConnSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// GitConnectionsStore is the store surface for provider connections. *store.DB satisfies it.
type GitConnectionsStore interface {
	CreateGitConnection(ctx context.Context, c store.GitConnection) error
	GetGitConnection(ctx context.Context, id string) (store.GitConnection, error)
	ListGitConnections(ctx context.Context, provider string) ([]store.GitConnection, error)
	FindGitConnectionByAccount(ctx context.Context, provider, host, accountID, clientID, excludeID string) (store.GitConnection, bool, error)
	RenameGitConnection(ctx context.Context, id, label string) error
	SetGitConnectionAccount(ctx context.Context, id, accountID, accountName, scopes string) error
	SetGitConnectionOAuthApp(ctx context.Context, id, instanceURL, clientID string) error
	SetDefaultGitConnection(ctx context.Context, id string) error
	DeleteGitConnection(ctx context.Context, id string) error
	SetGitSourceConnection(ctx context.Context, serviceName, connectionID string) error
	ListGitSourcesByConnection(ctx context.Context, connectionID string) ([]string, error)
	RelinkOrphanedGitSources(ctx context.Context, host, connectionID string) (int, error)
}

// GitLabAppClient is the surface internal/api needs from internal/gitlabapp.Client.
type GitLabAppClient interface {
	ExchangeCode(ctx context.Context, instanceURL, clientID, clientSecret, redirectURI, code string) (gitlabapp.Tokens, error)
	RefreshToken(ctx context.Context, instanceURL, clientID, clientSecret, refreshToken string) (gitlabapp.Tokens, error)
	RevokeToken(ctx context.Context, instanceURL, clientID, clientSecret, token string) error
	CurrentUser(ctx context.Context, instanceURL, accessToken string) (gitlabapp.Account, error)
	ListProjects(ctx context.Context, instanceURL, accessToken string, opts gitprovider.ListOptions) ([]gitlabapp.Project, gitprovider.ListResult, error)
	GetProject(ctx context.Context, instanceURL, accessToken string, projectID int64) (gitlabapp.Project, error)
	ListBranches(ctx context.Context, instanceURL, accessToken string, projectID int64) ([]gitlabapp.Branch, error)
	CreateProjectWebhook(ctx context.Context, instanceURL, accessToken string, projectID int64, hookURL, secretToken string) error
	CreateMergeRequestNote(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int, body string) (int64, error)
	ListMergeRequestNotes(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int) ([]gitprovider.Comment, error)
	UpdateMergeRequestNote(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int, noteID int64, body string) error
	CreateCommitStatus(ctx context.Context, instanceURL, accessToken, projectPath, sha string, state gitlabapp.CommitState, targetURL, description, name string) error
}

// BitbucketAppClient is the surface internal/api needs from internal/bitbucketapp.Client.
type BitbucketAppClient interface {
	ExchangeCode(ctx context.Context, key, secret, code string) (bitbucketapp.Tokens, error)
	RefreshToken(ctx context.Context, key, secret, refreshToken string) (bitbucketapp.Tokens, error)
	CurrentUser(ctx context.Context, accessToken string) (bitbucketapp.Account, error)
	ListRepos(ctx context.Context, accessToken string, opts gitprovider.ListOptions) ([]bitbucketapp.Repo, gitprovider.ListResult, error)
	GetRepo(ctx context.Context, accessToken, fullName string) (bitbucketapp.Repo, error)
	ListBranches(ctx context.Context, accessToken, fullName string) ([]bitbucketapp.Branch, error)
	CreateRepoWebhook(ctx context.Context, accessToken, fullName, hookURL, secret string) error
	CreatePullRequestComment(ctx context.Context, accessToken, fullName string, prID int, body string) (int64, error)
	ListPullRequestComments(ctx context.Context, accessToken, fullName string, prID int) ([]gitprovider.Comment, error)
	UpdatePullRequestComment(ctx context.Context, accessToken, fullName string, prID int, commentID int64, body string) error
	CreateCommitBuildStatus(ctx context.Context, accessToken, fullName, commit string, state bitbucketapp.BuildStatusState, targetURL, description, key string) error
}

// GiteaAppClient is the surface internal/api needs from internal/giteaapp.Client.
type GiteaAppClient interface {
	ExchangeCode(ctx context.Context, instanceURL, clientID, clientSecret, redirectURI, code string) (giteaapp.Tokens, error)
	RefreshToken(ctx context.Context, instanceURL, clientID, clientSecret, refreshToken string) (giteaapp.Tokens, error)
	CurrentUser(ctx context.Context, instanceURL, accessToken string) (giteaapp.Account, error)
	ListRepos(ctx context.Context, instanceURL, accessToken string, opts gitprovider.ListOptions) ([]giteaapp.Repo, gitprovider.ListResult, error)
	GetRepo(ctx context.Context, instanceURL, accessToken, fullName string) (giteaapp.Repo, error)
	ListBranches(ctx context.Context, instanceURL, accessToken, fullName string) ([]giteaapp.Branch, error)
	CreateRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName, hookURL, secret string) error
	CreateIssueComment(ctx context.Context, instanceURL, accessToken, fullName string, number int, body string) (int64, error)
	ListIssueComments(ctx context.Context, instanceURL, accessToken, fullName string, number int) ([]gitprovider.Comment, error)
	UpdateIssueComment(ctx context.Context, instanceURL, accessToken, fullName string, commentID int64, body string) error
	CreateCommitStatus(ctx context.Context, instanceURL, accessToken, fullName, sha string, state giteaapp.CommitStatusState, targetURL, description, statusContext string) error
}

// Secret env keys under a connection's secrets key. The Bitbucket client
// secret keeps the name its single-row connection already stored it under.
const (
	gitConnAccessTokenKey  = "access_token"
	gitConnRefreshTokenKey = "refresh_token"
	gitConnTokenExpiresKey = "token_expires_at"
	gitConnClientSecretKey = "client_secret"
	bitbucketSecretKey     = "secret"

	gitlabCallbackPath    = "/api/v1/gitlab-app/callback"
	bitbucketCallbackPath = "/api/v1/bitbucket-app/callback"
	giteaCallbackPath     = "/api/v1/gitea-app/callback"

	errGitMasterKeyRequired = "git provider connections: this requires a master key to be configured on this control plane"
)

type gitAccountInfo struct {
	ID    string
	Login string
}

type gitRepoInfo struct {
	CloneURL      string
	DefaultBranch string
}

type gitBranchInfo struct {
	Name      string
	CommitSHA string
}

// gitProviderAdapter is the provider-specific half of a connection: what
// differs between GitLab, Bitbucket and Gitea once the account is picked.
// repo identifies a repository: a numeric project id for GitLab, an
// "owner/name" path for the others.
type gitProviderAdapter struct {
	provider     string
	label        string
	secretKey    string
	callbackPath string
	scopes       string
	revokeNote   string
	authorizeURL func(c store.GitConnection, redirectURI, state string) string
	exchange     func(ctx context.Context, c store.GitConnection, secret, redirectURI, code string) (gitOAuthTokens, error)
	refresh      func(ctx context.Context, c store.GitConnection, secret, refreshToken string) (gitOAuthTokens, error)
	revoke       func(ctx context.Context, c store.GitConnection, secret, token string) error
	account      func(ctx context.Context, c store.GitConnection, token string) (gitAccountInfo, error)
	lookup       func(ctx context.Context, c store.GitConnection, token, repo string) (gitRepoInfo, error)
	branches     func(ctx context.Context, c store.GitConnection, token, repo string) ([]gitBranchInfo, error)
	webhook      func(ctx context.Context, c store.GitConnection, token, repo, hookURL, secret string) error
	// Set only by token-authenticated providers, which have no OAuth flow, refresh or client application.
	patAuth       bool
	deleteWebhook func(ctx context.Context, c store.GitConnection, token, repo, hookURL string) error
}

func (rt *Router) gitAdapter(provider string) (gitProviderAdapter, bool) {
	switch provider {
	case gitProviderGitLab:
		return rt.gitlabAdapter(), true
	case gitProviderBitbucket:
		return rt.bitbucketAdapter(), true
	case gitProviderGitea:
		return rt.giteaAdapter(), true
	case gitProviderForgejo:
		return rt.forgejoAdapter(), true
	case gitProviderBitbucketServer:
		return rt.bitbucketServerAdapter(), true
	}
	return gitProviderAdapter{}, false
}

func gitlabProjectID(repo string) (int64, error) {
	id, err := strconv.ParseInt(repo, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("repo must be a positive project id: %q", repo)
	}
	return id, nil
}

func (rt *Router) gitlabAdapter() gitProviderAdapter {
	c := rt.gitlabAppClient
	return gitProviderAdapter{
		provider: gitProviderGitLab, label: "GitLab", secretKey: gitConnClientSecretKey, callbackPath: gitlabCallbackPath, scopes: gitlabapp.DefaultScope,
		revokeNote: "Revoke it yourself under User Settings, Applications on your GitLab instance.",
		authorizeURL: func(conn store.GitConnection, redirectURI, state string) string {
			return gitlabapp.AuthorizeURL(conn.InstanceURL, conn.ClientID, redirectURI, state)
		},
		exchange: func(ctx context.Context, conn store.GitConnection, secret, redirectURI, code string) (gitOAuthTokens, error) {
			t, err := c.ExchangeCode(ctx, conn.InstanceURL, conn.ClientID, secret, redirectURI, code)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		refresh: func(ctx context.Context, conn store.GitConnection, secret, refresh string) (gitOAuthTokens, error) {
			t, err := c.RefreshToken(ctx, conn.InstanceURL, conn.ClientID, secret, refresh)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		revoke: func(ctx context.Context, conn store.GitConnection, secret, token string) error {
			return c.RevokeToken(ctx, conn.InstanceURL, conn.ClientID, secret, token)
		},
		account: func(ctx context.Context, conn store.GitConnection, token string) (gitAccountInfo, error) {
			a, err := c.CurrentUser(ctx, conn.InstanceURL, token)
			return gitAccountInfo{ID: a.ID, Login: a.Login}, err
		},
		lookup: func(ctx context.Context, conn store.GitConnection, token, repo string) (gitRepoInfo, error) {
			id, err := gitlabProjectID(repo)
			if err != nil {
				return gitRepoInfo{}, err
			}
			p, err := c.GetProject(ctx, conn.InstanceURL, token, id)
			return gitRepoInfo{CloneURL: p.HTTPURLToRepo, DefaultBranch: p.DefaultBranch}, err
		},
		branches: func(ctx context.Context, conn store.GitConnection, token, repo string) ([]gitBranchInfo, error) {
			id, err := gitlabProjectID(repo)
			if err != nil {
				return nil, err
			}
			bs, err := c.ListBranches(ctx, conn.InstanceURL, token, id)
			out := make([]gitBranchInfo, 0, len(bs))
			for _, b := range bs {
				out = append(out, gitBranchInfo{Name: b.Name, CommitSHA: b.CommitSHA})
			}
			return out, err
		},
		webhook: func(ctx context.Context, conn store.GitConnection, token, repo, hookURL, secret string) error {
			id, err := gitlabProjectID(repo)
			if err != nil {
				return err
			}
			return c.CreateProjectWebhook(ctx, conn.InstanceURL, token, id, hookURL, secret)
		},
	}
}

func (rt *Router) bitbucketAdapter() gitProviderAdapter {
	c := rt.bitbucketAppClient
	return gitProviderAdapter{
		provider: gitProviderBitbucket, label: "Bitbucket", secretKey: bitbucketSecretKey, callbackPath: bitbucketCallbackPath,
		revokeNote: "Bitbucket has no API to revoke an OAuth consumer's tokens. Revoke access under Personal settings, App authorizations on bitbucket.org.",
		authorizeURL: func(conn store.GitConnection, redirectURI, state string) string {
			return bitbucketapp.AuthorizeURL(conn.ClientID, redirectURI, state)
		},
		exchange: func(ctx context.Context, conn store.GitConnection, secret, _, code string) (gitOAuthTokens, error) {
			t, err := c.ExchangeCode(ctx, conn.ClientID, secret, code)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		refresh: func(ctx context.Context, conn store.GitConnection, secret, refresh string) (gitOAuthTokens, error) {
			t, err := c.RefreshToken(ctx, conn.ClientID, secret, refresh)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		account: func(ctx context.Context, _ store.GitConnection, token string) (gitAccountInfo, error) {
			a, err := c.CurrentUser(ctx, token)
			return gitAccountInfo{ID: a.ID, Login: a.Login}, err
		},
		lookup: func(ctx context.Context, _ store.GitConnection, token, repo string) (gitRepoInfo, error) {
			r, err := c.GetRepo(ctx, token, repo)
			return gitRepoInfo{CloneURL: r.CloneURL, DefaultBranch: r.DefaultBranch}, err
		},
		branches: func(ctx context.Context, _ store.GitConnection, token, repo string) ([]gitBranchInfo, error) {
			bs, err := c.ListBranches(ctx, token, repo)
			out := make([]gitBranchInfo, 0, len(bs))
			for _, b := range bs {
				out = append(out, gitBranchInfo{Name: b.Name, CommitSHA: b.CommitSHA})
			}
			return out, err
		},
		webhook: func(ctx context.Context, _ store.GitConnection, token, repo, hookURL, secret string) error {
			return c.CreateRepoWebhook(ctx, token, repo, hookURL, secret)
		},
	}
}

func (rt *Router) giteaAdapter() gitProviderAdapter {
	c := rt.giteaAppClient
	return gitProviderAdapter{
		provider: gitProviderGitea, label: "Gitea", secretKey: gitConnClientSecretKey, callbackPath: giteaCallbackPath, scopes: giteaapp.DefaultScope,
		revokeNote: "Gitea has no API to revoke an OAuth2 application's tokens. Remove the application under Settings, Applications on your Gitea instance.",
		authorizeURL: func(conn store.GitConnection, redirectURI, state string) string {
			return giteaapp.AuthorizeURL(conn.InstanceURL, conn.ClientID, redirectURI, state)
		},
		exchange: func(ctx context.Context, conn store.GitConnection, secret, redirectURI, code string) (gitOAuthTokens, error) {
			t, err := c.ExchangeCode(ctx, conn.InstanceURL, conn.ClientID, secret, redirectURI, code)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
		refresh: func(ctx context.Context, conn store.GitConnection, secret, refresh string) (gitOAuthTokens, error) {
			t, err := c.RefreshToken(ctx, conn.InstanceURL, conn.ClientID, secret, refresh)
			return gitOAuthTokens{Access: t.AccessToken, Refresh: t.RefreshToken, Expires: t.ExpiresAt}, err
		},
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
	}
}

// gitConnDefaultLabel is the label a connection gets before the operator renames it.
func gitConnDefaultLabel(provider, host, account string) string {
	switch {
	case account != "" && host != "" && provider != gitProviderBitbucket:
		return account + " @ " + host
	case account != "":
		return account
	case host != "":
		return host
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}
