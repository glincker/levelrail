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
// mintGitHubAppInstallationToken's sentinels, mapped to 409 by
// writeGitHubAppTokenError below.
var (
	errGitHubAppNotConnected = errors.New("api: github app is not connected")
	errGitHubAppNotInstalled = errors.New("api: github app is connected but not installed on any account")
)

// mintGitHubAppInstallationToken mints a fresh installation token on
// every call rather than caching one: installation tokens are
// short-lived (~1h) and this is low-frequency, human-driven traffic,
// so a cache would add invalidation complexity for little benefit.
// Returns the connection's own InstanceURL alongside the token: every
// caller needs it too, to keep talking to the same GitHub or GHES
// instance the App is actually registered on.
func (rt *Router) mintGitHubAppInstallationToken(ctx context.Context) (instanceURL, token string, err error) {
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		return "", "", errGitHubAppNotConnected
	}
	if err != nil {
		return "", "", fmt.Errorf("get github app connection: %w", err)
	}
	if conn.InstallationID == nil {
		return "", "", errGitHubAppNotInstalled
	}
	// Defensive, not expected in practice: migrations/0050 backfills
	// every pre-existing row to "https://github.com", and every writer
	// of a connection row (handleGitHubAppCallback,
	// handleConnectGitHubAppManually) always sets InstanceURL explicitly
	// now. An empty value here would otherwise build a bare
	// "/owner/repo.git" clone URL below.
	instanceURL = conn.InstanceURL
	if instanceURL == "" {
		instanceURL = "https://github.com"
	}

	privateKeyPEM, err := rt.githubAppSecrets.Resolve(ctx, store.GitHubAppSecretsKey(), "private_key")
	if err != nil {
		return "", "", fmt.Errorf("resolve github app private key: %w", err)
	}

	appJWT, err := githubapp.SignAppJWT(conn.AppID, []byte(privateKeyPEM), time.Now())
	if err != nil {
		return "", "", fmt.Errorf("sign github app jwt: %w", err)
	}

	tok, err := rt.githubAppClient.MintInstallationToken(ctx, instanceURL, appJWT, *conn.InstallationID)
	if err != nil {
		return "", "", fmt.Errorf("mint github app installation token: %w", err)
	}
	return instanceURL, tok.Token, nil
}

// writeGitHubAppTokenError maps mintGitHubAppInstallationToken's
// sentinels to 409, everything else to a logged 500.
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
	// CloneURL is derived from FullName, not passed through from
	// GitHub's response: the two are always equivalent.
	CloneURL string `json:"clone_url"`
}

func toGitHubAppRepoResource(r githubapp.Repo, instanceURL string) gitHubAppRepoResource {
	return gitHubAppRepoResource{
		FullName:      r.FullName,
		Name:          r.Name,
		OwnerLogin:    r.OwnerLogin,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      instanceURL + "/" + r.FullName + ".git",
	}
}

// handleListGitHubAppRepos handles GET /api/v1/github-app/repos: every
// repository the connected installation can access, for the frontend's
// repo picker. AbilityReadSensitive: this discloses real private-repo
// names, the same class GET .../backups/{id}/download already gates at
// that tier, not AbilityRoot (reserved for changing the connection
// itself).
func (rt *Router) handleListGitHubAppRepos(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	instanceURL, token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.writeGitHubAppTokenError(w, "api: mint github app installation token for repo listing failed", err)
		return
	}

	repos, err := rt.githubAppClient.ListInstallationRepos(ctx, instanceURL, token)
	if err != nil {
		rt.logger.Error("api: list github app installation repos failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to list repositories from github")
		return
	}

	out := make([]gitHubAppRepoResource, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toGitHubAppRepoResource(repo, instanceURL))
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
	instanceURL, token, err := rt.mintGitHubAppInstallationToken(ctx)
	if err != nil {
		rt.writeGitHubAppTokenError(w, "api: mint github app installation token for branch listing failed", err)
		return
	}

	branches, err := rt.githubAppClient.ListBranches(ctx, instanceURL, token, owner, repo)
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
