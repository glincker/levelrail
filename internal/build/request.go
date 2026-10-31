package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Request describes a single Dockerfile build. It is deliberately the
// pure, unit-testable input to newSolveOpt: nothing in this file touches
// a filesystem, a network, or a daemon, so Validate can be exercised with
// plain table-driven tests that need no live BuildKit connection.
type Request struct {
	// ContextDir is the build context root, matching `docker build <dir>`.
	// Required.
	ContextDir string

	// DockerfilePath is the path to the Dockerfile. It may live outside
	// ContextDir. Defaults to "<ContextDir>/Dockerfile" when empty.
	DockerfilePath string

	// RootDir, when set, is the checkout the build must stay inside:
	// ContextDir and the Dockerfile must resolve within it after symlinks.
	RootDir string

	// Tag is the image reference given to the built image, e.g.
	// "levelrail-spike:latest". Required.
	Tag string

	// Target is an optional multi-stage build target name.
	Target string

	// BuildArgs are passed through as build-time --build-arg equivalents.
	BuildArgs map[string]string

	// NoCache disables BuildKit's cache for this build.
	NoCache bool

	// Attest asks BuildKit for an SBOM and minimal provenance attestation.
	Attest bool

	// S3Cache adds BuildKit's s3 cache backend for this build. Nil means none.
	S3Cache *S3Cache

	// Secrets are BuildKit secret mounts by id, readable in a Dockerfile
	// with RUN --mount=type=secret,id=<id>. They never enter an image layer
	// or the build cache key. Local builds only: a remote build node does
	// not receive them.
	Secrets map[string][]byte

	// CgroupParent places the build's processes in a pre-sized cgroup so
	// CPU and memory limits hold. Set by the Limiter; local builds only.
	CgroupParent string
}

// ErrContextDirRequired and ErrTagRequired are returned by Validate for
// the two required fields, so callers can match on them if they need to
// distinguish the failure reason without parsing the error string.
var (
	ErrContextDirRequired = errors.New("build: context dir is required")
	ErrTagRequired        = errors.New("build: tag is required")
)

// Validate checks the request is well-formed enough to attempt a build.
// It does not check the filesystem; newSolveOpt does that, because a
// missing directory is an environment problem, not a malformed request.
func (r Request) Validate() error {
	if r.ContextDir == "" {
		return ErrContextDirRequired
	}
	if r.Tag == "" {
		return ErrTagRequired
	}
	return nil
}

// dockerfilePath resolves the effective Dockerfile path for this
// request, applying the "<ContextDir>/Dockerfile" default.
func (r Request) dockerfilePath() string {
	if r.DockerfilePath != "" {
		return r.DockerfilePath
	}
	return filepath.Join(r.ContextDir, "Dockerfile")
}

// EnsureWithin returns an error when target, after resolving symlinks,
// is not inside root. Paths that do not exist yet are checked through
// their nearest existing parent, so a missing root is not an error.
func EnsureWithin(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("resolve %q: %w", root, err)
	}
	realTarget, err := evalExisting(target)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", target, err)
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("%q resolves outside the repository checkout", target)
	}
	return nil
}

func evalExisting(p string) (string, error) {
	p = filepath.Clean(p)
	var rest []string
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			parts := append([]string{resolved}, rest...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(p)
		if !os.IsNotExist(err) || parent == p || strings.TrimSpace(p) == "" {
			return "", err
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}
