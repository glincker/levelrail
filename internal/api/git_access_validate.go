package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gitAccessTimeout = 20 * time.Second

	gitCheckPass = "pass"
	gitCheckFail = "fail"
	gitCheckSkip = "skipped"

	gitAuthDeployToken = "deploy_token"
	gitAuthGitHubApp   = "github_app"
	gitAuthNone        = "none"

	gitFixBadRepoURL    = "bad_repo_url"
	gitFixNoAuth        = "no_credential"
	gitFixAuthDenied    = "credential_denied"
	gitFixRepoNotFound  = "repo_not_found"
	gitFixBranchMissing = "branch_missing"
	gitFixUnreachable   = "unreachable"
	gitFixNotConnected  = "not_connected"
	gitFixNotInstalled  = "not_installed"
	gitFixKeyRejected   = "key_rejected"
)

var errRemoteBranchNotFound = errors.New("branch not found on the remote")

type gitAccessCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type gitAccessFix struct {
	Kind fixit.Kind `json:"kind"`
	Code string     `json:"code"`
	Hint string     `json:"hint"`
	Link string     `json:"link,omitempty"`
}

type gitAccessResult struct {
	OK         bool             `json:"ok"`
	Credential string           `json:"credential,omitempty"`
	Branch     string           `json:"branch,omitempty"`
	HeadSHA    string           `json:"head_sha,omitempty"`
	Checks     []gitAccessCheck `json:"checks"`
	Fix        *gitAccessFix    `json:"fix,omitempty"`
}

func (g *gitAccessResult) add(id, status, detail string) {
	g.Checks = append(g.Checks, gitAccessCheck{ID: id, Status: status, Detail: detail})
}

func (g *gitAccessResult) fail(id, detail string, fix gitAccessFix) {
	g.add(id, gitCheckFail, detail)
	g.Fix = &fix
}

// handleValidateGitSourceAccess handles POST /api/v1/apps/{name}/git-source/validate: it
// reads the repo's branch with the credential a deploy would use and says what to fix.
func (rt *Router) handleValidateGitSourceAccess(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: validate git source: load failed", err)
		return
	}
	res := gitAccessResult{Checks: []gitAccessCheck{}, Branch: gs.Branch}
	if err := build.ValidatePublicRepoURL(gs.RepoURL); err != nil {
		res.fail("repo_url", "the repository URL is not a valid https or git URL", gitAccessFix{
			Kind: fixit.KindManual, Code: gitFixBadRepoURL, Hint: "Edit the repository URL in the git source and use an https URL."})
		res.add("credential", gitCheckSkip, "skipped")
		res.add("access", gitCheckSkip, "skipped")
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.add("repo_url", gitCheckPass, "the repository URL is valid")

	token, cred := rt.gitAccessCredential(r.Context(), name, gs.RepoURL)
	res.Credential = cred
	res.add("credential", gitCheckPass, gitCredentialDetail(cred))

	ctx, cancel := context.WithTimeout(r.Context(), gitAccessTimeout)
	defer cancel()
	branch := gs.Branch
	if branch == "" {
		branch = "main"
	}
	sha, err := rt.resolveBranchSHA(ctx, gs.RepoURL, branch, token)
	if err != nil {
		rt.logger.Warn("api: validate git source: access failed", slog.String("app", name), slog.String("repo_url", redactURLCredentials(gs.RepoURL)), slog.String("kind", classifyGitAccessError(err, cred)))
		code := classifyGitAccessError(err, cred)
		detail, fix := gitAccessFailure(code, name, branch)
		res.fail("access", detail, fix)
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.HeadSHA = sha
	res.add("access", gitCheckPass, "the branch was read with this credential")
	res.OK = true
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) gitAccessCredential(ctx context.Context, name, repoURL string) (token, kind string) {
	if t := rt.storedGitSourceToken(ctx, name, repoURL); t != "" {
		return t, gitAuthDeployToken
	}
	if t := rt.tokenForRepo(ctx, name, repoURL, true); t != "" {
		return t, gitAuthGitHubApp
	}
	return "", gitAuthNone
}

func gitCredentialDetail(kind string) string {
	switch kind {
	case gitAuthDeployToken:
		return "a stored deploy token is used"
	case gitAuthGitHubApp:
		return "the connected GitHub App is used"
	}
	return "no credential applies, so only a public repository can be read"
}

func classifyGitAccessError(err error, cred string) string {
	switch {
	case errors.Is(err, errRemoteBranchNotFound):
		return gitFixBranchMissing
	case errors.Is(err, transport.ErrAuthorizationFailed):
		return gitFixAuthDenied
	case errors.Is(err, transport.ErrAuthenticationRequired):
		if cred == gitAuthNone {
			return gitFixNoAuth
		}
		return gitFixAuthDenied
	case errors.Is(err, transport.ErrRepositoryNotFound):
		if cred == gitAuthNone {
			return gitFixNoAuth
		}
		return gitFixRepoNotFound
	case errors.Is(err, context.DeadlineExceeded):
		return gitFixUnreachable
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "authentication") || strings.Contains(msg, "401") || strings.Contains(msg, "403") {
		return gitFixAuthDenied
	}
	return gitFixUnreachable
}

func gitAccessFailure(code, app, branch string) (string, gitAccessFix) {
	link := "/apps/" + app
	switch code {
	case gitFixNoAuth:
		return "the repository is private or does not exist, and no credential applies", gitAccessFix{
			Kind: fixit.KindGuided, Code: code, Link: "/settings/github-app",
			Hint: "Connect the GitHub App for this repository's owner, or add a deploy token to the git source."}
	case gitFixAuthDenied:
		return "the repository host rejected the credential", gitAccessFix{
			Kind: fixit.KindGuided, Code: code, Link: link,
			Hint: "The deploy token is expired or lacks read access. Replace it in the git source, or reinstall the GitHub App on this repository."}
	case gitFixRepoNotFound:
		return "the credential works but cannot see this repository", gitAccessFix{
			Kind: fixit.KindGuided, Code: code, Link: link,
			Hint: "Give the token or the GitHub App access to this repository, or correct the repository URL."}
	case gitFixBranchMissing:
		return "the branch " + branch + " does not exist on the remote", gitAccessFix{
			Kind: fixit.KindGuided, Code: code, Link: link, Hint: "Pick an existing branch in the git source."}
	}
	return "the repository host could not be reached", gitAccessFix{
		Kind: fixit.KindManual, Code: gitFixUnreachable, Hint: "Check the control plane's outbound network access and the host's status, then validate again."}
}

// handleValidateGitProvider handles POST /api/v1/git-providers/{provider}/validate: whether the
// provider connection holds a working credential right now.
func (rt *Router) handleValidateGitProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	res := gitAccessResult{Checks: []gitAccessCheck{}}
	page := "/settings/" + provider + "-app"
	switch provider {
	case "github":
		rt.validateGitHubProvider(r.Context(), &res, page)
	case "gitlab", "gitea", "bitbucket", "forgejo", "bitbucket_server":
		st := rt.gitProviderStatus(r.Context(), provider)
		if !st.Connected {
			res.fail("connection", "the provider is not connected or its authorization is missing", gitAccessFix{
				Kind: fixit.KindGuided, Code: gitFixNotConnected, Link: page, Hint: "Connect or re-authorize the provider."})
		} else {
			res.add("connection", gitCheckPass, "the provider is connected and authorized")
			res.OK = true
		}
	default:
		writeError(w, http.StatusNotFound, "unknown git provider")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) gitProviderStatus(ctx context.Context, provider string) gitProviderResource {
	return rt.gitProviderStatusFor(ctx, provider)
}

func (rt *Router) validateGitHubProvider(ctx context.Context, res *gitAccessResult, page string) {
	_, _, err := rt.mintGitHubAppInstallationToken(ctx)
	switch {
	case err == nil:
		res.add("connection", gitCheckPass, "the GitHub App is connected and installed")
		res.add("credentials", gitCheckPass, "the app key signs a token the installation accepts")
		res.OK = true
	case errors.Is(err, errGitHubAppNotConnected):
		res.fail("connection", "no GitHub App is connected", gitAccessFix{
			Kind: fixit.KindGuided, Code: gitFixNotConnected, Link: page, Hint: "Create or connect a GitHub App."})
	case errors.Is(err, errGitHubAppNotInstalled):
		res.add("connection", gitCheckPass, "the GitHub App is connected")
		res.fail("installation", "the app is not installed on any account", gitAccessFix{
			Kind: fixit.KindGuided, Code: gitFixNotInstalled, Link: page, Hint: "Install the GitHub App on your account or organization."})
	default:
		rt.logger.Warn("api: validate github app: token mint failed", slog.String("error", err.Error()))
		res.add("connection", gitCheckPass, "the GitHub App is connected")
		res.fail("credentials", "GitHub rejected the app key or the installation", gitAccessFix{
			Kind: fixit.KindGuided, Code: gitFixKeyRejected, Link: page, Hint: "Reconnect the GitHub App so a fresh private key is stored."})
	}
}
