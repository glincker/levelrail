package build

import (
	"errors"
	"path/filepath"
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

	// Tag is the image reference given to the built image, e.g.
	// "levelrail-spike:latest". Required.
	Tag string

	// Target is an optional multi-stage build target name.
	Target string

	// BuildArgs are passed through as build-time --build-arg equivalents.
	BuildArgs map[string]string

	// NoCache disables BuildKit's cache for this build.
	NoCache bool
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
