package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/giteaapp"
	"github.com/glincker/levelrail/internal/gitlabapp"
	"github.com/glincker/levelrail/internal/gitprovider"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

const (
	gitAccessDefaultTTL   = 60 * time.Second
	gitAccessDefaultNames = 25

	gitAuthOAuthApp = "oauth_app"

	gitCapAllowed      = "allowed"
	gitCapNotAllowed   = "not_allowed"
	gitCapNeedsScope   = "needs_scope"
	gitScopesFromHost  = "provider"
	gitScopesFromStore = "stored"
)

// Capability ids of the access view, in display order.
const (
	capListRepos        = "list_repos"
	capCloneSync        = "clone_sync"
	capRegisterWebhook  = "register_webhook"
	capTagPushTrigger   = "tag_push_trigger"
	capCommitStatus     = "commit_status"
	capMRComments       = "mr_comments"
	capDeploymentsReprt = "deployments_report"
	capRegistryPull     = "registry_pull"
	capImportGroup      = "import_group"
)

// gitAccessCapability is one thing Levelrail does with a connection and whether the token allows it.
type gitAccessCapability struct {
	ID    string `json:"id"`
	State string `json:"state"`
	// NeedsScope is the scope that would fix a needs_scope state.
	NeedsScope string `json:"needs_scope,omitempty"`
	// Verified is true only when the state is proven from the provider's own scope report.
	Verified bool   `json:"verified"`
	Note     string `json:"note,omitempty"`
}

type gitAccessNames struct {
	Names     []string `json:"names"`
	Total     int      `json:"total"`
	Truncated bool     `json:"truncated"`
}

// gitAccessResource is the wire shape of GET /api/v1/git-connections/{id}/access. It never carries a credential.
type gitAccessResource struct {
	ID           string                `json:"id"`
	Provider     string                `json:"provider"`
	AuthMethod   string                `json:"auth_method"`
	InstanceURL  string                `json:"instance_url,omitempty"`
	AccountName  string                `json:"account_name,omitempty"`
	Scopes       []string              `json:"scopes"`
	ScopesKnown  bool                  `json:"scopes_known"`
	ScopesSource string                `json:"scopes_source,omitempty"`
	ExpiresAt    string                `json:"expires_at,omitempty"`
	LastUsedAt   string                `json:"last_used_at,omitempty"`
	Visible      gitAccessNames        `json:"visible_accounts"`
	Capabilities []gitAccessCapability `json:"capabilities"`
	CheckedAt    string                `json:"checked_at"`
	giteaProbe   *giteaapp.Access
	Cached       bool   `json:"cached"`
	LiveCheckErr string `json:"live_check_error,omitempty"`
}

type gitAccessCacheEntry struct {
	at  time.Time
	res gitAccessResource
}

var (
	gitAccessCacheMu sync.Mutex
	gitAccessCache   = map[string]gitAccessCacheEntry{}
)

// gitlabAccessProber is the optional GitLab surface the access view uses beyond GitLabAppClient.
type gitlabAccessProber interface {
	TokenInfo(ctx context.Context, instanceURL, accessToken string) (gitlabapp.TokenInfo, error)
	OAuthTokenInfo(ctx context.Context, instanceURL, accessToken string) (gitlabapp.TokenInfo, error)
	ListGroups(ctx context.Context, instanceURL, accessToken string, limit int) ([]string, bool, error)
}

func (rt *Router) gitAccessTTL(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return gitAccessDefaultTTL
	}
	return time.Duration(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyGitAccessCacheTTL)) * time.Second
}

func (rt *Router) gitAccessNamesMax(ctx context.Context) int {
	if rt.runtimeSettings == nil {
		return gitAccessDefaultNames
	}
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyGitAccessNamesMax)
}

// handleGetGitConnectionAccess handles GET /api/v1/git-connections/{id}/access:
// what the connection's credential can do. A provider failure still answers 200
// with the stored facts and live_check_error.
func (rt *Router) handleGetGitConnectionAccess(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadGitConnection(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ttl := rt.gitAccessTTL(ctx)
	gitAccessCacheMu.Lock()
	if e, hit := gitAccessCache[c.ID]; hit && time.Since(e.at) < ttl {
		gitAccessCacheMu.Unlock()
		res := e.res
		res.Cached = true
		writeJSON(w, http.StatusOK, res)
		return
	}
	gitAccessCacheMu.Unlock()

	res, live := rt.buildGitAccess(ctx, c)
	if live {
		gitAccessCacheMu.Lock()
		gitAccessCache[c.ID] = gitAccessCacheEntry{at: time.Now(), res: res}
		gitAccessCacheMu.Unlock()
	}
	writeJSON(w, http.StatusOK, res)
}

// buildGitAccess merges stored facts with one live provider check; live reports whether that check succeeded.
func (rt *Router) buildGitAccess(ctx context.Context, c store.GitConnection) (gitAccessResource, bool) {
	res := gitAccessResource{
		ID: c.ID, Provider: c.Provider, AuthMethod: gitAuthOAuthApp, InstanceURL: c.InstanceURL, AccountName: c.AccountName,
		Scopes: []string{}, Visible: gitAccessNames{Names: []string{}}, CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	patAuth := c.ClientID == ""
	if patAuth {
		res.AuthMethod = gitAuthToken
	}
	if stored := strings.Fields(strings.ReplaceAll(c.Scopes, ",", " ")); len(stored) > 0 && c.Provider == gitProviderGitLab {
		res.Scopes, res.ScopesKnown, res.ScopesSource = stored, true, gitScopesFromStore
	}

	err := rt.liveGitAccess(ctx, c, patAuth, &res)
	if err != nil {
		res.LiveCheckErr = err.Error()
	}
	res.Capabilities = gitCapabilities(c.Provider, res.Scopes, res.ScopesKnown, res.ScopesSource == gitScopesFromHost)
	if res.giteaProbe != nil {
		res.Capabilities = giteaCapabilities(*res.giteaProbe)
	}
	return res, err == nil
}

func (rt *Router) liveGitAccess(ctx context.Context, c store.GitConnection, patAuth bool, res *gitAccessResource) error {
	switch c.Provider {
	case gitProviderGitLab, gitProviderBitbucket, gitProviderGitea, gitProviderForgejo:
	default:
		return errors.New("live check is not supported for this provider")
	}
	ctx, cancel := context.WithTimeout(ctx, gitProbeTimeout)
	defer cancel()
	_, token, err := rt.gitConnToken(ctx, c.ID)
	if err != nil {
		return errors.New("no usable credential: " + gitAccessErrText(err))
	}
	limit := rt.gitAccessNamesMax(ctx)
	if c.Provider == gitProviderGitLab {
		return rt.liveGitLabAccess(ctx, c, token, patAuth, limit, res)
	}
	var names []string
	var trunc bool
	switch c.Provider {
	case gitProviderBitbucket:
		rs, lr, lerr := rt.bitbucketAppClient.ListRepos(ctx, token, gitprovider.ListOptions{MaxItems: gitTokenRepoSample})
		if lerr != nil {
			return errors.New("list repositories: " + gitAccessErrText(lerr))
		}
		for _, r := range rs {
			names = append(names, r.FullName)
		}
		trunc = lr.Truncated
	default:
		if handled, perr := rt.liveGiteaAccess(ctx, c, token, limit, res); handled {
			return perr
		}
		rs, lr, lerr := rt.giteaAppClient.ListRepos(ctx, c.InstanceURL, token, gitprovider.ListOptions{MaxItems: gitTokenRepoSample})
		if lerr != nil {
			return errors.New("list repositories: " + gitAccessErrText(lerr))
		}
		for _, r := range rs {
			names = append(names, r.FullName)
		}
		trunc = lr.Truncated
	}
	res.Visible = ownerNames(names, limit, trunc)
	return nil
}

func (rt *Router) liveGitLabAccess(ctx context.Context, c store.GitConnection, token string, patAuth bool, limit int, res *gitAccessResource) error {
	pr, ok := rt.gitlabAppClient.(gitlabAccessProber)
	if !ok {
		return errors.New("live check is not available")
	}
	var info gitlabapp.TokenInfo
	var terr error
	if patAuth {
		info, terr = pr.TokenInfo(ctx, c.InstanceURL, token)
	} else {
		info, terr = pr.OAuthTokenInfo(ctx, c.InstanceURL, token)
	}
	if terr == nil {
		res.Scopes, res.ScopesKnown, res.ScopesSource = info.Scopes, true, gitScopesFromHost
		res.ExpiresAt, res.LastUsedAt = info.ExpiresAt, info.LastUsedAt
	}
	groups, more, gerr := pr.ListGroups(ctx, c.InstanceURL, token, limit)
	if gerr == nil {
		res.Visible = gitAccessNames{Names: groups, Total: len(groups), Truncated: more}
	}
	switch {
	case terr != nil:
		return errors.New("token info: " + gitAccessErrText(terr))
	case gerr != nil:
		return errors.New("list groups: " + gitAccessErrText(gerr))
	}
	return nil
}

// gitAccessErrText keeps provider error text short; clients already strip credentials from their errors.
func gitAccessErrText(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ownerNames reduces repository full names to distinct owners (accounts or workspaces).
func ownerNames(fullNames []string, limit int, truncated bool) gitAccessNames {
	seen := map[string]bool{}
	var owners []string
	for _, n := range fullNames {
		o, _, _ := strings.Cut(n, "/")
		if o != "" && !seen[o] {
			seen[o] = true
			owners = append(owners, o)
		}
	}
	slices.Sort(owners)
	out := gitAccessNames{Names: owners, Total: len(owners), Truncated: truncated}
	if len(owners) > limit {
		out.Names, out.Truncated = owners[:limit], true
	}
	return out
}

func hasAnyScope(have []string, want ...string) bool {
	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}
	return false
}

// gitCapabilities maps granted scopes to what Levelrail does with a connection.
// proven is true when the scope list came from the provider itself.
func gitCapabilities(provider string, scopes []string, known, proven bool) []gitAccessCapability {
	if provider == gitProviderGitLab && known {
		return gitlabCapabilities(scopes, proven)
	}
	inferred := func(id string) gitAccessCapability {
		return gitAccessCapability{ID: id, State: gitCapAllowed, Note: "scopes are not reported by this provider; assumed from the connection type"}
	}
	unsupported := func(id string) gitAccessCapability {
		return gitAccessCapability{ID: id, State: gitCapNotAllowed, Note: "not supported for this provider yet"}
	}
	out := []gitAccessCapability{
		inferred(capListRepos), inferred(capCloneSync), inferred(capRegisterWebhook), inferred(capTagPushTrigger),
		inferred(capCommitStatus), inferred(capMRComments), unsupported(capDeploymentsReprt), unsupported(capRegistryPull), unsupported(capImportGroup),
	}
	if provider == gitProviderGitLab {
		for i := range out {
			if out[i].State == gitCapAllowed {
				out[i].Note = "the stored scopes are unknown; assumed from the connection type"
			}
		}
		out[6] = gitAccessCapability{ID: capDeploymentsReprt, State: gitCapAllowed, Note: "scopes unknown; assumed from the connection type. Reporting is off until git.gitlab_deployments is on"}
		out[7] = inferred(capRegistryPull)
		out[8] = inferred(capImportGroup)
	}
	return out
}

func gitlabCapabilities(scopes []string, proven bool) []gitAccessCapability {
	api := hasAnyScope(scopes, "api")
	mk := func(id string, ok bool, fix string, explicit bool, note string) gitAccessCapability {
		c := gitAccessCapability{ID: id, Note: note}
		if ok {
			c.State = gitCapAllowed
			c.Verified = proven && explicit
		} else {
			c.State, c.NeedsScope, c.Verified = gitCapNeedsScope, fix, proven
		}
		return c
	}
	const role = "also needs Maintainer access on the project, which scopes cannot show"
	return []gitAccessCapability{
		mk(capListRepos, hasAnyScope(scopes, "api", "read_api"), "read_api", true, ""),
		mk(capCloneSync, hasAnyScope(scopes, "read_repository", "write_repository"), "read_repository", true, ""),
		mk(capRegisterWebhook, api, "api", false, role),
		mk(capTagPushTrigger, api, "api", false, role),
		mk(capCommitStatus, api, "api", false, "also needs Developer access on the project, which scopes cannot show"),
		mk(capMRComments, api, "api", false, "also needs Reporter access on the project, which scopes cannot show"),
		mk(capDeploymentsReprt, api, "api", false, "also needs Developer access on the project, which scopes cannot show. Reports to Deployments and Environments when git.gitlab_deployments is on"),
		mk(capRegistryPull, hasAnyScope(scopes, "read_registry", "api"), "read_registry", hasAnyScope(scopes, "read_registry"), ""),
		mk(capImportGroup, hasAnyScope(scopes, "api", "read_api"), "read_api", true, ""),
	}
}
