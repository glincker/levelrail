package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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
	// Defensive, not expected in practice: migrations/0061 backfills
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

// mintGitHubAppInstallationTokenForOwner resolves which installation
// covers ownerLogin (case-insensitively) instead of always using the
// legacy single installation_id. Falls back to
// mintGitHubAppInstallationToken when no installation matches.
func (rt *Router) mintGitHubAppInstallationTokenForOwner(ctx context.Context, ownerLogin string) (instanceURL, token string, err error) {
	installations, listErr := rt.githubApp.ListGitHubAppInstallations(ctx)
	if listErr != nil {
		return "", "", fmt.Errorf("list github app installations: %w", listErr)
	}
	var matched *store.GitHubAppInstallation
	for i := range installations {
		if strings.EqualFold(installations[i].AccountLogin, ownerLogin) {
			matched = &installations[i]
			break
		}
	}
	if matched == nil {
		return rt.mintGitHubAppInstallationToken(ctx)
	}

	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		return "", "", errGitHubAppNotConnected
	}
	if err != nil {
		return "", "", fmt.Errorf("get github app connection: %w", err)
	}
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
	tok, err := rt.githubAppClient.MintInstallationToken(ctx, instanceURL, appJWT, matched.InstallationID)
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
	// AccountType groups the picker's card grid by account (migrations/0282):
	// "user" for the admin's own personal account, "organization" otherwise.
	AccountType string `json:"account_type"`
}

func toGitHubAppRepoResource(r githubapp.Repo, instanceURL, accountType string) gitHubAppRepoResource {
	return gitHubAppRepoResource{
		FullName:      r.FullName,
		Name:          r.Name,
		OwnerLogin:    r.OwnerLogin,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      instanceURL + "/" + r.FullName + ".git",
		AccountType:   accountType,
	}
}

// gitHubAppRepoListResource is GET /api/v1/github-app/repos's response:
// repos from every connected installation, plus one error per
// installation that failed so the frontend can show a per-group error
// badge (handleListGitHubAppRepos's own doc comment) instead of losing
// every other group's results to one bad installation.
type gitHubAppRepoListResource struct {
	Repos  []gitHubAppRepoResource        `json:"repos"`
	Errors []gitHubAppRepoListErrResource `json:"errors,omitempty"`
}

type gitHubAppRepoListErrResource struct {
	AccountLogin string `json:"account_login"`
	Error        string `json:"error"`
}

// handleListGitHubAppRepos handles GET /api/v1/github-app/repos: every
// repository every connected installation can access, for the frontend's
// repo picker. AbilityReadSensitive, not AbilityRoot: this discloses
// repo names, not the connection itself. Fans out one call per
// installation concurrently; a failing installation is captured in
// Errors rather than failing the other installations' results.
func (rt *Router) handleListGitHubAppRepos(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	installations, err := rt.githubApp.ListGitHubAppInstallations(ctx)
	if err != nil {
		rt.logger.Error("api: list github app installations for repo listing failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(installations) == 0 {
		// Legacy fallback: a connection whose single installation_id
		// predates migrations/0282's backfill condition (shouldn't
		// happen after that migration runs, kept defensive).
		instanceURL, token, tokErr := rt.mintGitHubAppInstallationToken(ctx)
		if tokErr != nil {
			rt.writeGitHubAppTokenError(w, "api: mint github app installation token for repo listing failed", tokErr)
			return
		}
		repos, listErr := rt.githubAppClient.ListInstallationRepos(ctx, instanceURL, token)
		if listErr != nil {
			rt.logger.Error("api: list github app installation repos failed", slog.String("error", listErr.Error()))
			writeError(w, http.StatusBadGateway, "failed to list repositories from github")
			return
		}
		out := make([]gitHubAppRepoResource, 0, len(repos))
		for _, repo := range repos {
			out = append(out, toGitHubAppRepoResource(repo, instanceURL, "organization"))
		}
		writeJSON(w, http.StatusOK, gitHubAppRepoListResource{Repos: out})
		return
	}

	type result struct {
		accountLogin string
		accountType  string
		repos        []gitHubAppRepoResource
		err          error
	}
	results := make([]result, len(installations))
	var wg sync.WaitGroup
	for i, inst := range installations {
		wg.Add(1)
		go func(i int, inst store.GitHubAppInstallation) {
			defer wg.Done()
			instanceURL, token, tokErr := rt.mintGitHubAppInstallationTokenForOwner(ctx, inst.AccountLogin)
			if tokErr != nil {
				results[i] = result{accountLogin: inst.AccountLogin, accountType: inst.AccountType, err: tokErr}
				return
			}
			repos, listErr := rt.githubAppClient.ListInstallationRepos(ctx, instanceURL, token)
			if listErr != nil {
				results[i] = result{accountLogin: inst.AccountLogin, accountType: inst.AccountType, err: listErr}
				return
			}
			out := make([]gitHubAppRepoResource, 0, len(repos))
			for _, repo := range repos {
				out = append(out, toGitHubAppRepoResource(repo, instanceURL, inst.AccountType))
			}
			results[i] = result{accountLogin: inst.AccountLogin, accountType: inst.AccountType, repos: out}
		}(i, inst)
	}
	wg.Wait()

	resp := gitHubAppRepoListResource{Repos: []gitHubAppRepoResource{}}
	for _, res := range results {
		if res.err != nil {
			rt.logger.Warn("api: list repos for one github app installation failed",
				slog.String("account_login", res.accountLogin), slog.String("error", res.err.Error()))
			resp.Errors = append(resp.Errors, gitHubAppRepoListErrResource{AccountLogin: res.accountLogin, Error: res.err.Error()})
			continue
		}
		resp.Repos = append(resp.Repos, res.repos...)
	}
	writeJSON(w, http.StatusOK, resp)
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
