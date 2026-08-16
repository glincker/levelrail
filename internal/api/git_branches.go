package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
)

// errRepoURLSchemeNotAllowed is returned by listRemoteBranches when
// repoURL's scheme isn't http/https. go-git's transport client registry
// (github.com/go-git/go-git/v5/plumbing/transport/client) registers a
// "file" transport unconditionally alongside http/https/ssh/git
// (confirmed by reading that package: not a blank-import side effect,
// a direct top-level import in client.go), so an unrestricted repoURL
// would let a caller read local git repos on the control-plane host via
// a file:// URL, or probe internal network reachability via http(s)://
// against an internal address. This route's own contract is "a public
// repo only" (see handleListGitBranches's doc comment); this is what
// actually enforces that rather than just documenting it.
var errRepoURLSchemeNotAllowed = fmt.Errorf("repo_url must use http or https")

// listBranchesFunc lists every branch name a git remote advertises, with
// no clone and no local checkout: exactly what `git ls-remote` does. A
// seam (the same "overridable in tests, defaults to a real
// implementation" shape fetchFunc already establishes for
// handleTriggerBuild) so dispatch-logic tests don't need real network
// access.
type listBranchesFunc func(ctx context.Context, repoURL string) ([]string, error)

// listRemoteBranches is the real listBranchesFunc implementation: an
// in-memory remote (memory.NewStorage(), never touching disk, since
// nothing here is ever checked out) advertised-refs listing, filtered
// down to branch refs and sorted for a stable response. No auth is
// attempted, matching this route's own scope: a public repo only, the
// same input a caller would otherwise hand-type into repo_url on
// POST /api/v1/apps/{name}/builds.
func listRemoteBranches(ctx context.Context, repoURL string) ([]string, error) {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("api: list branches: %q: %w", repoURL, err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("api: list branches: %q: %w", repoURL, errRepoURLSchemeNotAllowed)
	}
	return listRemoteBranchesUnchecked(ctx, repoURL)
}

// listRemoteBranchesUnchecked is listRemoteBranches's scheme-gate-free
// core: the actual go-git advertised-refs listing and branch filtering,
// with no restriction on repoURL's scheme. Never called directly from
// handler code; kept separate purely so tests can exercise the listing
// and filtering logic against a local temp-dir repo (a bare filesystem
// path, no scheme) without a real network dependency, the same
// tradeoff TestListRemoteBranches_Live's own doc comment already
// documents, while listRemoteBranches itself stays the only path a real
// caller-supplied repoURL can ever reach.
func listRemoteBranchesUnchecked(ctx context.Context, repoURL string) ([]string, error) {
	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})

	refs, err := remote.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("api: list branches: %q: %w", repoURL, err)
	}

	branches := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !ref.Name().IsBranch() {
			continue
		}
		branches = append(branches, ref.Name().Short())
	}
	sort.Strings(branches)
	return branches, nil
}

// gitBranchesRequest is POST /api/v1/git/branches's body: repo_url only,
// a body rather than a query param since repo_url can be long and can
// carry characters (colons, slashes, an embedded credential-free
// userinfo) a query string would need to escape, the same reasoning
// triggerBuildRequest.RepoURL is already a JSON body field rather than a
// path or query parameter.
type gitBranchesRequest struct {
	RepoURL string `json:"repo_url"`
}

// gitBranchesResponse is POST /api/v1/git/branches's success body.
type gitBranchesResponse struct {
	Branches []string `json:"branches"`
}

// handleListGitBranches handles POST /api/v1/git/branches: every branch
// name a public git remote advertises, so the create-app-from-git
// wizard (web/src/components/CreateAppFromGitFields.tsx) can offer a
// real picker instead of a bare free-text ref field. Gated at
// AbilityDeploy, the same tier POST /api/v1/apps/{name}/builds itself
// requires (router.go): listing what a repo could be built from is a
// strict subset of what actually triggering that build already lets a
// caller do, so it earns no looser a gate.
//
// A private or unreachable repo is a real, expected error case, not a
// bug: it fails as 400 with a caller-actionable message, never a 500,
// since nothing about this control plane is broken when a given URL
// simply isn't a reachable public remote.
func (rt *Router) handleListGitBranches(w http.ResponseWriter, r *http.Request) {
	var req gitBranchesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}

	branches, err := rt.listBranches(r.Context(), req.RepoURL)
	if err != nil {
		rt.logger.Warn("api: list git branches failed", slog.String("error", err.Error()), slog.String("repo_url", req.RepoURL))
		writeError(w, http.StatusBadRequest, "could not list branches, confirm the repository is public and the URL is correct")
		return
	}

	writeJSON(w, http.StatusOK, gitBranchesResponse{Branches: branches})
}
