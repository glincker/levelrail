package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// errGitSourceKindConflict keeps a compose source and a single-service source off one app.
var errGitSourceKindConflict = errors.New("this app already has a git source of the other kind (compose or single service); disconnect it first")

const defaultComposeRepoPath = "compose.yaml"

// maxComposeRepoPathBytes bounds a stored path; it is a format limit, not a tunable.
const maxComposeRepoPathBytes = 512

type setComposeSourceRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch,omitempty"`
	ComposePath string `json:"compose_path,omitempty"`
	Token       string `json:"token,omitempty"`
	TriggerMode string `json:"trigger_mode,omitempty"`
}

// validateComposeRepoPath accepts only a clean repo-relative .yml or .yaml path.
func validateComposeRepoPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return defaultComposeRepoPath, nil
	}
	if len(p) > maxComposeRepoPathBytes {
		return "", fmt.Errorf("compose_path must be at most %d bytes", maxComposeRepoPathBytes)
	}
	for _, c := range p {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) {
			return "", errors.New("compose_path must not contain control or format characters")
		}
	}
	if strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return "", errors.New("compose_path must be a path relative to the repository root")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errors.New("compose_path must stay inside the repository (no ..)")
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", errors.New("compose_path must name a file")
	}
	if ext := path.Ext(clean); ext != ".yml" && ext != ".yaml" {
		return "", errors.New("compose_path must end in .yml or .yaml")
	}
	return clean, nil
}

// readRepoComposeFile reads rel through os.Root, so a symlink cannot leave the checkout.
func (rt *Router) readRepoComposeFile(ctx context.Context, dir, rel string) ([]byte, error) {
	clean, err := validateComposeRepoPath(rel)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open checkout: %w", err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.FromSlash(clean))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("compose file %s not found in the repository", clean)
		}
		return nil, fmt.Errorf("compose file %s cannot be read inside the repository", clean)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("compose file %s is not a regular file", clean)
	}
	limit := int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyComposeRepoFileMaxBytes))
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("compose file %s is larger than %d bytes", clean, limit)
	}
	return data, nil
}

// checkRepoComposeKeys refuses keys that would pull other repo files or need a build;
// the pasted compose path never resolves them, so a repo file must not rely on them.
func checkRepoComposeKeys(body []byte) error {
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil //nolint:nilerr // the compose parser reports the syntax error with its own message
	}
	if _, ok := doc["include"]; ok {
		return errors.New("include: is not supported for a compose file from git; inline the included services")
	}
	services, _ := doc["services"].(map[string]any)
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		if _, ok := svc["build"]; ok {
			return fmt.Errorf("service %q: build: is not supported for a compose app from git; set image: to a pre-built image instead", name)
		}
		if ext, ok := svc["extends"].(map[string]any); ok {
			if _, other := ext["file"]; other {
				return fmt.Errorf("service %q: extends: from another file is not supported; inline the keys", name)
			}
		}
	}
	return nil
}

// fetchRepoCompose checks out ref and returns the configured compose file.
// Submodules and LFS stay off: only one file is read.
func (rt *Router) fetchRepoCompose(ctx context.Context, name string, gs store.GitSource, ref string) ([]byte, int, string) {
	body, _, status, msg := rt.fetchRepoComposeMeta(ctx, name, gs, ref)
	return body, status, msg
}

// checkoutCommitOrder reads sha's first parent and commit time from the checkout, so a
// deploy-now cursor can refuse an older late push; zero values when it is not a repository.
func checkoutCommitOrder(dir, sha string) (before string, at time.Time) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: false})
	if err != nil {
		return "", time.Time{}
	}
	c, err := repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", time.Time{}
	}
	if len(c.ParentHashes) > 0 {
		before = c.ParentHashes[0].String()
	}
	return before, c.Committer.When
}

func (rt *Router) fetchRepoComposeMeta(ctx context.Context, name string, gs store.GitSource, ref string) ([]byte, store.DeployOrder, int, string) {
	order := store.DeployOrder{Automatic: true, CommitSHA: ref}
	if rt.gitSourceFetch == nil {
		return nil, order, http.StatusNotImplemented, "git deploys are not configured on this control plane"
	}
	token, scope, err := rt.resolveGitSourceDeployCredential(ctx, name)
	if err != nil {
		rt.logger.Error("api: compose git source: resolve deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil, order, http.StatusInternalServerError, "internal error"
	}
	dir, cleanup, err := rt.gitSourceFetch(sourcecache.WithScope(ctx, scope), gs.RepoURL, ref, token)
	if err != nil {
		rt.logger.Error("api: compose git source: fetch failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("ref", ref))
		if errors.Is(err, errGitCommitNotFound) {
			return nil, order, http.StatusUnprocessableEntity, "commit not found"
		}
		if errors.Is(err, sourcecache.ErrHostUnreachable) {
			return nil, order, http.StatusServiceUnavailable, sourcecache.ErrHostUnreachable.Error()
		}
		return nil, order, http.StatusInternalServerError, "deploy failed"
	}
	defer cleanup()
	order.Before, order.CommitAt = checkoutCommitOrder(dir, ref)
	body, err := rt.readRepoComposeFile(ctx, dir, gs.ComposePath)
	if err != nil {
		return nil, order, http.StatusUnprocessableEntity, err.Error()
	}
	if err := checkRepoComposeKeys(body); err != nil {
		return nil, order, http.StatusUnprocessableEntity, err.Error()
	}
	return body, order, 0, ""
}

// composeMembers returns a compose app's members, or ErrServiceNotFound when name is not one.
func (rt *Router) composeMembers(ctx context.Context, appID string) ([]store.DesiredService, error) {
	if rt.appGroups == nil {
		return nil, store.ErrServiceNotFound
	}
	members, err := rt.appGroups.ListServicesByApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 || members[0].Name == appID {
		return nil, store.ErrServiceNotFound
	}
	return members, nil
}

func (rt *Router) loadComposeGitSource(w http.ResponseWriter, r *http.Request, name string) (*store.GitSource, bool) {
	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) || (err == nil && gs.ComposePath == "") {
		writeError(w, http.StatusNotFound, "no compose git source connected for this app")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: compose git source: load failed", err, slog.String("name", name))
		return nil, false
	}
	return gs, true
}

// handleGetComposeSource handles GET /api/v1/apps/{name}/compose-source.
func (rt *Router) handleGetComposeSource(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	gs, ok := rt.loadComposeGitSource(w, r, name)
	if !ok {
		return
	}
	hasToken, err := rt.gitSourceSecrets.Exists(r.Context(), store.GitSourceSecretsKey(name), gitSourceTokenKey)
	if err != nil {
		rt.internalError(w, "api: compose git source: check token failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.gitSourceResourceFor(r.Context(), *gs, hasToken, name))
}

// handleSetComposeSource handles PUT /api/v1/apps/{name}/compose-source: deploy an
// existing compose app from a repo file. The webhook secret is shown once, on create.
func (rt *Router) handleSetComposeSource(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	var req setComposeSourceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if prev, err := rt.gitSources.GetGitSource(r.Context(), name); err == nil && prev.ComposePath == "" {
		writeError(w, http.StatusConflict, errGitSourceKindConflict.Error())
		return
	} else if err != nil && !errors.Is(err, store.ErrGitSourceNotFound) {
		rt.internalError(w, "api: compose git source: load existing failed", err, slog.String("name", name))
		return
	}
	if _, err := rt.composeMembers(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "compose app not found: deploy the stack once, then connect its repository")
		return
	} else if err != nil {
		rt.internalError(w, "api: compose git source: list services failed", err, slog.String("name", name))
		return
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil && !gitsshkey.IsSSHURL(req.RepoURL) {
		writeError(w, http.StatusBadRequest, "repo_url must use http, https or ssh")
		return
	}
	composePath, err := validateComposeRepoPath(req.ComposePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	triggerMode, err := normalizeGitSourceTriggerMode(req.TriggerMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "trigger_mode for a compose source must be push or release: "+err.Error())
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = webhook.DefaultBranch
	}
	result, err := rt.connectGitSource(withRequestOrigin(r.Context(), r), name, connectGitSourceParams{
		RepoURL: req.RepoURL, Branch: branch, BuildType: spec.BuildCompose, Token: req.Token,
		TriggerMode: triggerMode, ComposePath: composePath,
	})
	if errors.Is(err, errGitSourceKindConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: compose git source: connect failed", err, slog.String("name", name))
		return
	}
	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: compose git source connected", slog.String("name", name), slog.String("repo_url", redactURLCredentials(req.RepoURL)), slog.String("compose_path", composePath))
	writeJSON(w, status, result.Resource)
}

// handleDeployComposeSource handles POST /api/v1/apps/{name}/compose-source/deploy:
// deploy the branch head now as the caller, so the caller's own abilities gate it.
func (rt *Router) handleDeployComposeSource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	gs, ok := rt.loadComposeGitSource(w, r, name)
	if !ok {
		return
	}
	_, protected, err := rt.protectedEnvironmentOf(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: compose git source: check protected environment failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusServiceUnavailable, "could not check whether this app needs deploy approval, retry later")
		return
	}
	if protected {
		writeError(w, http.StatusConflict, "this app is in a protected environment: push to the branch so the deploy goes through approval")
		return
	}
	token, _, err := rt.resolveGitSourceDeployCredential(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: compose git source: resolve token failed", err, slog.String("name", name))
		return
	}
	sha, err := rt.resolveBranchSHA(rt.gitAuthCtx(r.Context(), name, gs.RepoURL), gs.RepoURL, gs.Branch, token)
	if err != nil {
		rt.logger.Warn("api: compose git source: resolve branch failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not resolve the branch head of the repository")
		return
	}
	mu := composeGitLock(name)
	mu.Lock()
	defer mu.Unlock()
	attemptID, saved := rt.saveComposeAttempt(r.Context(), name, sha, store.DeployAttemptSourceManual)
	if !saved {
		rt.internalError(w, "api: compose git source: save attempt failed", errors.New("save attempt"), slog.String("name", name))
		return
	}
	body, order, status, msg := rt.fetchRepoComposeMeta(r.Context(), name, *gs, sha)
	if status != 0 {
		rt.finishComposeAttempt(r.Context(), attemptID, store.DeployAttemptStatusFailed, msg)
		writeError(w, status, strings.TrimSpace(msg))
		return
	}
	resp, ok := rt.deployComposeBody(w, r, name, body, composeDeployOpts{source: store.ComposeRevisionSourceGit})
	if !ok {
		rt.finishComposeAttempt(r.Context(), attemptID, store.DeployAttemptStatusFailed, "compose deploy refused")
		return
	}
	// Deploy-now advances the cursor too, so a later stale push is judged against this commit.
	if cursors, _ := rt.deploySafety.(composeCursorStore); cursors != nil {
		if err := cursors.AdvanceDeployCursor(r.Context(), name, order); err != nil {
			rt.finishComposeAttempt(r.Context(), attemptID, store.DeployAttemptStatusFailed, "commit could not be recorded")
			rt.internalError(w, "api: compose git source: advance cursor failed", err, slog.String("name", name))
			return
		}
	}
	rt.finishComposeAttempt(r.Context(), attemptID, store.DeployAttemptStatusSucceeded, "")
	writeJSON(w, http.StatusOK, resp)
}
