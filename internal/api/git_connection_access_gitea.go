package api

import (
	"context"
	"errors"
	"time"

	"github.com/glincker/levelrail/internal/giteaapp"
	"github.com/glincker/levelrail/internal/gitprovider"
	"github.com/glincker/levelrail/internal/store"
)

// giteaAccessProber is optional on GiteaAppClient so fakes without it keep
// working and fall back to the inferred matrix.
type giteaAccessProber interface {
	ProbeAccess(ctx context.Context, instanceURL, accessToken string, opts gitprovider.ListOptions) (giteaapp.Access, error)
}

// liveGiteaAccess fills the access view from the forge's own per-repository
// permissions, since Gitea and Forgejo report no token scopes.
func (rt *Router) liveGiteaAccess(ctx context.Context, c store.GitConnection, token string, limit int, res *gitAccessResource) (bool, error) {
	pr, ok := rt.giteaAppClient.(giteaAccessProber)
	if !ok {
		return false, nil
	}
	info, err := pr.ProbeAccess(ctx, c.InstanceURL, token, gitprovider.ListOptions{MaxItems: gitTokenRepoSample})
	if err != nil {
		return true, errors.New("probe access: " + gitAccessErrText(err))
	}
	res.Visible = gitAccessNames{Names: info.Owners, Total: len(info.Owners), Truncated: info.Truncated}
	if len(info.Owners) > limit {
		res.Visible.Names, res.Visible.Truncated = info.Owners[:limit], true
	}
	// Only OAuth tokens expire; a personal token has no expiry on these forges.
	if exp := gitUnixSecret(rt.gitSecret(ctx, c.SecretsKey, gitConnTokenExpiresKey)); !exp.IsZero() {
		res.ExpiresAt = exp.Format(time.RFC3339)
	}
	res.giteaProbe = &info
	return true, nil
}

// giteaCapabilities derives each capability from what the server said the
// token may do on real repositories; with no repository sampled nothing is
// claimed as verified.
func giteaCapabilities(a giteaapp.Access) []gitAccessCapability {
	seen := a.Sampled > 0
	mk := func(id string, ok bool, need, note string) gitAccessCapability {
		c := gitAccessCapability{ID: id, Note: note, Verified: seen}
		if ok {
			c.State = gitCapAllowed
		} else {
			c.State, c.NeedsScope = gitCapNeedsScope, need
		}
		return c
	}
	const writeNote = "needs write access on the repository, read from the repository permissions"
	const adminNote = "needs admin access on the repository, read from the repository permissions"
	notYet := func(id string) gitAccessCapability {
		return gitAccessCapability{ID: id, State: gitCapNotAllowed, Note: "not supported for this provider yet"}
	}
	groups := mk(capImportGroup, len(a.Orgs) > 0, "organization membership", "organizations are listed with the token's organization read access")
	groups.Verified = true
	return []gitAccessCapability{
		mk(capListRepos, true, "", ""),
		mk(capCloneSync, a.CanPull || !seen, "repository read", ""),
		mk(capRegisterWebhook, a.CanAdmin || !seen, "repository admin", adminNote),
		mk(capTagPushTrigger, a.CanAdmin || !seen, "repository admin", adminNote),
		mk(capCommitStatus, a.CanPush || !seen, "repository write", writeNote),
		mk(capMRComments, a.CanPush || !seen, "repository write", writeNote),
		notYet(capDeploymentsReprt),
		notYet(capRegistryPull),
		groups,
	}
}
