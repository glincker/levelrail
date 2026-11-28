package giteaapp

import (
	"context"
	"slices"

	"github.com/glincker/levelrail/internal/gitprovider"
)

// Access is what a token can really do, read from the server's own
// per-repository permissions because Gitea reports no token scopes.
type Access struct {
	Login     string
	Owners    []string
	Orgs      []string
	Sampled   int
	Truncated bool
	CanPull   bool
	CanPush   bool
	CanAdmin  bool
}

// ProbeAccess samples the repositories the token can reach and folds their
// permission flags into one summary, so the capability matrix reflects the
// forge instead of the connection type.
func (c *Client) ProbeAccess(ctx context.Context, instanceURL, accessToken string, opts gitprovider.ListOptions) (Access, error) {
	acct, err := c.CurrentUser(ctx, instanceURL, accessToken)
	if err != nil {
		return Access{}, err
	}
	repos, res, err := c.ListRepos(ctx, instanceURL, accessToken, opts)
	if err != nil {
		return Access{}, err
	}
	out := Access{Login: acct.Login, Sampled: len(repos), Truncated: res.Truncated, CanPull: len(repos) > 0}
	for _, r := range repos {
		out.CanPush = out.CanPush || r.Push || r.Admin
		out.CanAdmin = out.CanAdmin || r.Admin
		if r.Owner != "" && !slices.Contains(out.Owners, r.Owner) {
			out.Owners = append(out.Owners, r.Owner)
		}
	}
	// Orgs are best effort: a token without read:organization still lists repos.
	if orgs, oerr := c.ListOrgs(ctx, instanceURL, accessToken); oerr == nil {
		out.Orgs = orgs
		for _, o := range orgs {
			if !slices.Contains(out.Owners, o) {
				out.Owners = append(out.Owners, o)
			}
		}
	}
	slices.Sort(out.Owners)
	return out, nil
}
