package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// errGitHubAppNotConnected and errGitHubAppNotInstalled are
// mintGitHubAppInstallationToken's own sentinels, so its two callers
// (handleListGitHubAppRepos, handleListGitHubAppBranches) can each map
// them to the same 409 response without duplicating the
// errors.Is(store.ErrGitHubAppConnectionNotFound) / "InstallationID ==
// nil" checks twice.
var (
	errGitHubAppNotConnected = errors.New("api: github app is not connected")
	errGitHubAppNotInstalled = errors.New("api: github app is connected but not installed on any account")
)

// mintGitHubAppInstallationToken mints a fresh installation access
// token for this control plane's connected App, on every call: unlike a
// user session or an API token, installation tokens are short-lived
// (~1 hour per GitHub's docs) and this package deliberately does not
// build expiry tracking to cache and reuse one across requests, since
// repo/branch listing is low-frequency, interactive, human-driven
// traffic (a settings-page repo picker, not a hot path), and minting one
// per request is simpler and unambiguously correct where a cache would
// need its own invalidation logic for comparatively little benefit.
//
// The private key is decrypted (rt.githubAppSecrets.Resolve) and held
// only in the appJWT/privateKeyPEM local variables for the duration of
// this call; nothing here persists it, logs it, or returns it.
func (rt *Router) mintGitHubAppInstallationToken(ctx context.Context) (string, error) {
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		return "", errGitHubAppNotConnected
	}
	if err != nil {
		return "", fmt.Errorf("get github app connection: %w", err)
	}
	if conn.InstallationID == nil {
		return "", errGitHubAppNotInstalled
	}

	privateKeyPEM, err := rt.githubAppSecrets.Resolve(ctx, store.GitHubAppSecretsKey(), "private_key")
	if err != nil {
		return "", fmt.Errorf("resolve github app private key: %w", err)
	}

	appJWT, err := githubapp.SignAppJWT(conn.AppID, []byte(privateKeyPEM), time.Now())
	if err != nil {
		return "", fmt.Errorf("sign github app jwt: %w", err)
	}

	tok, err := rt.githubAppClient.MintInstallationToken(ctx, appJWT, *conn.InstallationID)
	if err != nil {
		return "", fmt.Errorf("mint github app installation token: %w", err)
	}
	return tok.Token, nil
}

// writeGitHubAppTokenError maps mintGitHubAppInstallationToken's
// sentinel errors to the 409 responses handleListGitHubAppRepos and
// handleListGitHubAppBranches both need, and any other error to a
// logged 500. Shared so the two handlers can't drift on wording or
// status codes for the same two well-known states.
func (rt *Router) writeGitHubAppTokenError(w http.ResponseWriter, logMsg string, err error) {
	if errors.Is(err, errGitHubAppNotConnected) {
		writeError(w, http.StatusConflict, "no github app is connected")
		return
	}
	if errors.Is(err, errGitHubAppNotInstalled) {
		writeError(w, http.StatusConflict, "the github app is connected but not installed on any account yet")
		return
	}
	rt.logger.Error(logMsg, slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, "internal error")
}

// gitHubAppRepoResource is one entry of GET /api/v1/github-app/repos.
type gitHubAppRepoResource struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	OwnerLogin    string `json:"owner_login"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	// CloneURL is constructed from FullName
	// ("https://github.com/"+full_name+".git") rather than passed
	// through from GitHub's own repository object: the two are
	// equivalent for every real GitHub repository, and building it here
	// keeps internal/githubapp.Repo (and the GitHub API response it
	// decodes) from needing an extra field this control plane would
	// otherwise have to keep in sync for no behavioral difference.
	CloneURL string `json:"clone_url"`
}

func toGitHubAppRepoResource(r githubapp.Repo) gitHubAppRepoResource {
	return gitHubAppRepoResource{
		FullName:      r.FullName,
		Name:          r.Name,
		OwnerLogin:    r.OwnerLogin,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      "https://github.com/" + r.FullName + ".git",
	}
}

// handleListGitHubAppRepos handles GET /api/v1/github-app/repos: every
// repository the connected installation can access, for the frontend's
// repo picker (feeding CreateAppFromGitFields.tsx's repoUrl field).
//
// AbilityReadSensitive, not AbilityRead and not AbilityRoot: this
// returns real content about a connected external account (which
// private repositories exist, their names), the same "actual content,
// not metadata, but not a mutation" class GET .../backups/{id}/download
// already draws that exact line for (see handleDownloadBackup's own
// doc comment) -- closer to that than to AbilityRoot, which this
// package otherwise reserves for changing the connection itself
// (register/disconnect), a materially different, platform-wide-
// configuration risk class than reading a list of repo names through an
// already-established connection.
func (rt *Router) handleListGitHubAppRepos(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.writeGitHubAppTokenError(w, "api: mint github app installation token for repo listing failed", err)
		return
	}

	repos, err := rt.githubAppClient.ListInstallationRepos(ctx, token)
	if err != nil {
		rt.logger.Error("api: list github app installation repos failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list repositories from github")
		return
	}

	out := make([]gitHubAppRepoResource, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toGitHubAppRepoResource(repo))
	}
	writeJSON(w, http.StatusOK, out)
}

// gitHubAppBranchResource is one entry of
// GET /api/v1/github-app/repos/{owner}/{repo}/branches.
type gitHubAppBranchResource struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

// handleListGitHubAppBranches handles
// GET /api/v1/github-app/repos/{owner}/{repo}/branches, the same
// AbilityReadSensitive tier as handleListGitHubAppRepos above and for
// the same reasoning.
func (rt *Router) handleListGitHubAppBranches(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}

	ctx := r.Context()
	token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.writeGitHubAppTokenError(w, "api: mint github app installation token for branch listing failed", err)
		return
	}

	branches, err := rt.githubAppClient.ListBranches(ctx, token, owner, repo)
	if err != nil {
		rt.logger.Error("api: list github app repo branches failed", slog.String("error", err.Error()), slog.String("owner", owner), slog.String("repo", repo))
		writeError(w, http.StatusBadGateway, "failed to list branches from github")
		return
	}

	out := make([]gitHubAppBranchResource, 0, len(branches))
	for _, b := range branches {
		out = append(out, gitHubAppBranchResource{Name: b.Name, CommitSHA: b.CommitSHA})
	}
	writeJSON(w, http.StatusOK, out)
}
