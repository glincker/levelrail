package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	gitPushDefaultBranch = "main"
	gitPushUsername      = "git"
	gitPushTokenPrefix   = "gp_"
	gitPushRoutePrefix   = "/api/v1/git/"
	gitPushMaxBranchLen  = 100
)

// GitPushStore is the persistence the git push remote needs.
type GitPushStore interface {
	GetGitPushRemote(ctx context.Context, name string) (*store.GitPushRemote, error)
	SaveGitPushRemoteToken(ctx context.Context, name, tokenHash, defaultBranch string, now time.Time) error
	SetGitPushRemoteBranch(ctx context.Context, name, branch string) error
	RecordGitPush(ctx context.Context, name, commit, result string, at time.Time) error
	DeleteGitPushRemote(ctx context.Context, name string) error
}

// gitPushRuntime holds the in-memory limiters and per-app push locks.
type gitPushRuntime struct {
	once  sync.Once
	rate  *apiRateLimiter
	fails *loginLimiter
	mu    sync.Mutex
	busy  map[string]bool
}

func (g *gitPushRuntime) init() {
	g.once.Do(func() {
		g.rate = newAPIRateLimiter(envInt(envGitPushRatePerMinute, defaultGitPushRatePerMinute))
		g.fails = newLoginLimiter()
		g.busy = make(map[string]bool)
	})
}

func (g *gitPushRuntime) acquire(name string) bool {
	g.init()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[name] {
		return false
	}
	g.busy[name] = true
	return true
}

func (g *gitPushRuntime) release(name string) {
	g.mu.Lock()
	delete(g.busy, name)
	g.mu.Unlock()
}

type gitRemoteResource struct {
	Configured     bool       `json:"configured"`
	URL            string     `json:"url"`
	RemoteName     string     `json:"remote_name"`
	Username       string     `json:"username"`
	Branch         string     `json:"branch"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	RotatedAt      *time.Time `json:"rotated_at,omitempty"`
	LastPushAt     *time.Time `json:"last_push_at,omitempty"`
	LastPushCommit string     `json:"last_push_commit,omitempty"`
	LastPushResult string     `json:"last_push_result,omitempty"`
	MaxPushBytes   int64      `json:"max_push_bytes"`
}

type gitRemoteIssued struct {
	gitRemoteResource
	// Token is shown once; only its hash is stored.
	Token      string `json:"token"`
	AddCommand string `json:"add_command"`
}

type gitRemoteBranchRequest struct {
	Branch string `json:"branch"`
}

func newGitPushToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("api: mint git push token: %w", err)
	}
	return gitPushTokenPrefix + hex.EncodeToString(b), nil
}

func validGitPushBranch(b string) bool {
	if b == "" || len(b) > gitPushMaxBranchLen || strings.HasPrefix(b, "-") || strings.HasPrefix(b, "refs/") {
		return false
	}
	return plumbing.NewBranchReferenceName(b).Validate() == nil
}

func nonZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (rt *Router) gitRemoteURL(ctx context.Context, name string) string {
	path := gitPushRoutePrefix + name + ".git"
	base, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if origin := requestOriginFrom(ctx); origin != "" {
			return origin + path
		}
		return path
	}
	return base + path
}

func (rt *Router) gitRemoteName() string {
	if rt.brand != nil && rt.brand.ShortName != "" {
		return strings.ToLower(rt.brand.ShortName)
	}
	return "deploy"
}

func (rt *Router) gitRemoteResourceFor(ctx context.Context, name string, rm *store.GitPushRemote) gitRemoteResource {
	res := gitRemoteResource{
		URL:          rt.gitRemoteURL(ctx, name),
		RemoteName:   rt.gitRemoteName(),
		Username:     gitPushUsername,
		Branch:       gitPushDefaultBranch,
		MaxPushBytes: gitPushLimits().maxPushBytes,
	}
	if rm != nil {
		res.Configured = true
		res.Branch = rm.Branch
		res.CreatedAt = nonZeroTime(rm.CreatedAt)
		res.RotatedAt = nonZeroTime(rm.RotatedAt)
		res.LastPushAt = nonZeroTime(rm.LastPushAt)
		res.LastPushCommit = rm.LastPushCommit
		res.LastPushResult = rm.LastPushResult
	}
	return res
}

// gitRemoteAddCommand embeds the token in the URL so one paste works; it is
// printed once, when the token is issued.
func gitRemoteAddCommand(remoteName, rawURL, token string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "git remote add " + remoteName + " " + rawURL
	}
	u.User = url.UserPassword(gitPushUsername, token)
	return "git remote add " + remoteName + " " + u.String()
}

func (rt *Router) gitPushReady(w http.ResponseWriter, r *http.Request, name string) bool {
	if rt.gitPushStore == nil {
		writeError(w, http.StatusNotImplemented, "git push is not configured on this control plane")
		return false
	}
	return rt.requireAppExists(w, r, name)
}

func (rt *Router) requireAppExists(w http.ResponseWriter, r *http.Request, name string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "api: git remote: load app failed", err, slog.String("name", name))
		}
		return false
	}
	return true
}

// handleGetGitRemote reports the app's git push remote without its token.
func (rt *Router) handleGetGitRemote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.gitPushReady(w, r, name) {
		return
	}
	rm, err := rt.gitPushStore.GetGitPushRemote(r.Context(), name)
	if err != nil && !errors.Is(err, store.ErrGitPushRemoteNotFound) {
		rt.internalError(w, "api: get git remote failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.gitRemoteResourceFor(r.Context(), name, rm))
}

// handleIssueGitRemote creates the remote or rotates its token; the old token stops working at once.
func (rt *Router) handleIssueGitRemote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.gitPushReady(w, r, name) {
		return
	}
	token, err := newGitPushToken()
	if err != nil {
		rt.internalError(w, "api: issue git remote token failed", err)
		return
	}
	if err := rt.gitPushStore.SaveGitPushRemoteToken(r.Context(), name, tokenhash.Sum(token), gitPushDefaultBranch, time.Now()); err != nil {
		rt.internalError(w, "api: save git remote failed", err)
		return
	}
	rm, err := rt.gitPushStore.GetGitPushRemote(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: reload git remote failed", err)
		return
	}
	res := rt.gitRemoteResourceFor(r.Context(), name, rm)
	writeJSON(w, http.StatusOK, gitRemoteIssued{
		gitRemoteResource: res,
		Token:             token,
		AddCommand:        gitRemoteAddCommand(res.RemoteName, res.URL, token),
	})
}

// handleSetGitRemoteBranch changes which pushed branch deploys.
func (rt *Router) handleSetGitRemoteBranch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.gitPushReady(w, r, name) {
		return
	}
	var req gitRemoteBranchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if !validGitPushBranch(branch) {
		writeError(w, http.StatusBadRequest, "branch must be a plain branch name such as main")
		return
	}
	if err := rt.gitPushStore.SetGitPushRemoteBranch(r.Context(), name, branch); err != nil {
		if errors.Is(err, store.ErrGitPushRemoteNotFound) {
			writeError(w, http.StatusNotFound, "git push is not enabled for this app")
			return
		}
		rt.internalError(w, "api: set git remote branch failed", err)
		return
	}
	rm, err := rt.gitPushStore.GetGitPushRemote(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: reload git remote failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.gitRemoteResourceFor(r.Context(), name, rm))
}

// handleDeleteGitRemote revokes the remote and its token.
func (rt *Router) handleDeleteGitRemote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.gitPushReady(w, r, name) {
		return
	}
	if err := rt.gitPushStore.DeleteGitPushRemote(r.Context(), name); err != nil {
		rt.internalError(w, "api: delete git remote failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
