package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gitTokenRepoSample = 100
	gitTokenRevokeNote = "Delete the token yourself in your account's token settings on the host."
)

// gitTokenRequiredScopes are the scopes a token needs when the host can report them.
var gitTokenRequiredScopes = map[string][]string{
	gitProviderGitLab: {"api"},
}

// gitTokenDetails is what a host says a token can do.
type gitTokenDetails struct {
	Scopes       []string `json:"scopes,omitempty"`
	ScopesKnown  bool     `json:"scopes_known"`
	Missing      []string `json:"missing_scopes,omitempty"`
	RepoAccess   bool     `json:"repo_access"`
	RepoCount    int      `json:"repo_count"`
	RepoCountCap bool     `json:"repo_count_capped,omitempty"`
}

type gitlabTokenInfoer interface {
	TokenInfo(ctx context.Context, instanceURL, accessToken string) (gitlabapp.TokenInfo, error)
}

// inspectGitToken asks the host what a token may do. Hosts that cannot report scopes leave ScopesKnown false.
func (rt *Router) inspectGitToken(ctx context.Context, provider string, conn store.GitConnection, token string) (gitTokenDetails, error) {
	var d gitTokenDetails
	opts := gitprovider.ListOptions{MaxItems: gitTokenRepoSample}
	var res gitprovider.ListResult
	var n int
	var err error
	switch provider {
	case gitProviderGitLab:
		if ti, ok := rt.gitlabAppClient.(gitlabTokenInfoer); ok {
			if info, terr := ti.TokenInfo(ctx, conn.InstanceURL, token); terr == nil {
				d.Scopes, d.ScopesKnown = info.Scopes, true
			}
		}
		ps, r, lerr := rt.gitlabAppClient.ListProjects(ctx, conn.InstanceURL, token, opts)
		res, err, n = r, lerr, len(ps)
	case gitProviderBitbucket:
		rs, r, lerr := rt.bitbucketAppClient.ListRepos(ctx, token, opts)
		res, err, n = r, lerr, len(rs)
	case gitProviderGitea, gitProviderForgejo:
		rs, r, lerr := rt.giteaAppClient.ListRepos(ctx, conn.InstanceURL, token, opts)
		res, err, n = r, lerr, len(rs)
	default:
		return d, fmt.Errorf("token inspection is not supported for %s", provider)
	}
	if err != nil {
		return d, fmt.Errorf("list repositories: %w", err)
	}
	d.RepoAccess, d.RepoCount, d.RepoCountCap = n > 0, n, res.Truncated
	if d.ScopesKnown {
		d.Missing = missingGitScopes(provider, d.Scopes)
	}
	return d, nil
}

func missingGitScopes(provider string, have []string) []string {
	set := make(map[string]bool, len(have))
	for _, s := range have {
		set[s] = true
	}
	var out []string
	for _, want := range gitTokenRequiredScopes[provider] {
		if !set[want] {
			out = append(out, want)
		}
	}
	return out
}

func joinGitScopes(scopes []string) string { return strings.Join(scopes, " ") }
