package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/railwayapp/railpack/core"
	rpapp "github.com/railwayapp/railpack/core/app"
)

// detectCloneTimeout bounds the whole clone+inspect call: a pre-flight
// check the create-app wizard waits on synchronously, not a real build,
// so it must fail fast against an unreachable host or an oversized
// history rather than hang the wizard.
const detectCloneTimeout = 20 * time.Second

// detectMaxRepoBytes caps the checked-out tree size, checked after the
// clone completes: a defense-in-depth measure alongside Depth: 1 below,
// since a single commit can still contain a large blob (e.g. committed
// binaries or datasets) that a depth-1 clone does not bound on its own.
const detectMaxRepoBytes = 256 * 1024 * 1024

// ErrRepoTooLarge is returned by Detect when the cloned tree exceeds
// detectMaxRepoBytes.
var ErrRepoTooLarge = errors.New("build: detect: repository is too large to inspect")

// ErrRepoURLSchemeNotAllowed is returned by ValidatePublicRepoURL for a
// repoURL whose scheme is not http/https.
var ErrRepoURLSchemeNotAllowed = errors.New("repo_url must use http or https")

// ValidatePublicRepoURL rejects any scheme other than http/https, the
// same restriction internal/api/git_branches.go always enforced for a
// caller-supplied repo_url (that file's own doc comment on
// errRepoURLSchemeNotAllowed explains why: go-git registers a "file"
// transport unconditionally, and an unrestricted URL would let a caller
// read local files on this host or probe internal network reachability).
// Detect and internal/api/build_detect.go share this exact check since
// both accept the same caller-controlled repo_url shape.
func ValidatePublicRepoURL(repoURL string) error {
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("build: detect: %q: %w", repoURL, err)
	}
	switch u.Scheme {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("build: detect: %q: %w", repoURL, ErrRepoURLSchemeNotAllowed)
	}
}

// DetectRequest is Detect's input: a git source to inspect, never built
// or pushed anywhere.
type DetectRequest struct {
	// RepoURL is the git remote to shallow-clone. Required, and must pass
	// ValidatePublicRepoURL.
	RepoURL string
	// Ref is the branch to check out. Empty means the remote's default
	// branch (HEAD).
	Ref string
}

// DetectResult is Detect's output.
type DetectResult struct {
	// Provider is Railpack's own raw provider id (e.g. "node", "golang"),
	// empty if Railpack detected nothing buildable at all.
	Provider string
	// FrameworkName is FrameworkLabel(Provider)'s human-readable name,
	// empty if Provider is empty or not in supportedRailpackProviders:
	// either way, the caller's own contract is "fall back to manual build
	// type selection" for an empty FrameworkName, not an error.
	FrameworkName string
}

// Detect shallow-clones req.RepoURL at req.Ref into a temporary
// directory and runs Railpack's own provider detection against it, no
// build, no image, no BuildKit connection: the same pure
// core.GenerateBuildPlan call generateRailpackPlan (railpack.go) already
// uses for a real build, which only ever reads files under the checkout
// (package.json, go.mod, pom.xml, and the like) to decide a provider, so
// this never executes anything the checkout itself contains.
//
// Bounded against an untrusted repoURL three ways: a hard timeout
// (detectCloneTimeout), a shallow, single-branch clone (Depth: 1), and a
// post-clone size check (detectMaxRepoBytes). None of these are a
// build's own real safety net (the reconciler and BuildKit's own
// resource limits still apply once a build is actually triggered); they
// exist only because this pre-flight check runs synchronously inside an
// HTTP request the wizard is waiting on.
func Detect(ctx context.Context, req DetectRequest) (*DetectResult, error) {
	if err := ValidatePublicRepoURL(req.RepoURL); err != nil {
		return nil, err
	}
	return detectUnchecked(ctx, req)
}

// detectUnchecked is Detect's scheme-gate-free core, kept separate
// purely so tests can exercise the clone/inspect logic against a local
// temp-dir repo (a bare filesystem path, no scheme) without a real
// network dependency, the same tradeoff internal/api/git_branches.go's
// listRemoteBranchesUnchecked already documents for the identical split.
// Never called directly from a real caller; Detect is the only path a
// caller-supplied RepoURL can ever reach.
func detectUnchecked(ctx context.Context, req DetectRequest) (*DetectResult, error) {
	ctx, cancel := context.WithTimeout(ctx, detectCloneTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "levelrail-detect-*")
	if err != nil {
		return nil, fmt.Errorf("build: detect: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cloneOpts := &git.CloneOptions{
		URL:          req.RepoURL,
		Depth:        1,
		SingleBranch: true,
	}
	if req.Ref != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(req.Ref)
	}
	if _, err := git.PlainCloneContext(ctx, dir, false, cloneOpts); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("build: detect: clone %q: %w", req.RepoURL, ctx.Err())
		}
		return nil, fmt.Errorf("build: detect: clone %q: %w", req.RepoURL, err)
	}

	size, err := dirSize(dir, detectMaxRepoBytes)
	if err != nil {
		return nil, err
	}
	if size > detectMaxRepoBytes {
		return nil, ErrRepoTooLarge
	}

	app, err := rpapp.NewApp(dir)
	if err != nil {
		return nil, fmt.Errorf("build: detect: read source dir: %w", err)
	}
	env := rpapp.NewEnvironment(nil)

	result, err := core.GenerateBuildPlan(app, env, &core.GenerateBuildPlanOptions{RailpackVersion: "levelrail"})
	if err != nil {
		return nil, fmt.Errorf("build: detect: generate build plan: %w", err)
	}
	if !result.Success || len(result.DetectedProviders) == 0 {
		return &DetectResult{}, nil
	}

	provider := result.DetectedProviders[0]
	frameworkName, _ := FrameworkLabel(provider)
	return &DetectResult{Provider: provider, FrameworkName: frameworkName}, nil
}

// dirSize sums file sizes under dir, stopping as soon as the running
// total exceeds maxBytes: an oversized repo (the case this exists to
// catch) is caught in whatever directory happens to push it over,
// without walking the rest of a tree already known too large.
func dirSize(dir string, maxBytes int64) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		total += info.Size()
		if total > maxBytes {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("build: detect: measure checkout size: %w", err)
	}
	return total, nil
}
