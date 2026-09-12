package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// RemoteKind selects which solve path a dispatched build runs, since a
// Dockerfile build and a Railpack build are genuinely different solves
// (see railpack.go) rather than one solve with a flag.
type RemoteKind string

// RemoteKindDockerfile and RemoteKindRailpack are the two dispatchable
// build kinds, matching internal/deploy's own two building build types.
const (
	RemoteKindDockerfile RemoteKind = "dockerfile"
	RemoteKindRailpack   RemoteKind = "railpack"
)

// ErrRemoteDockerfileOutsideContext is returned when a build's Dockerfile
// sits outside its own build context. A local build can read it anyway,
// since both are paths on one filesystem; a dispatched one cannot,
// because only the context is shipped to the build node.
var ErrRemoteDockerfileOutsideContext = errors.New("build: a dispatched build's dockerfile must live inside its build context")

// RemoteRequest is one build dispatched to another node: everything the
// build node needs that is not the context's bytes.
type RemoteRequest struct {
	Kind RemoteKind

	// ContextDir is the build context root on whichever side currently
	// holds it: the control plane's checkout when dispatching, the build
	// node's unpacked copy when running.
	ContextDir string

	// DockerfilePath is relative to ContextDir, since the control plane's
	// own absolute paths mean nothing on the build node. Empty means the
	// context root's own Dockerfile. Ignored for RemoteKindRailpack.
	DockerfilePath string

	Tag       string
	Target    string
	BuildArgs map[string]string
	NoCache   bool

	// Cache is the registry cache backend the build node should import
	// from and export to. CacheConfig.Dir is never carried here: it names
	// a directory on the dispatching side's own disk.
	Cache CacheConfig
}

// NewRemoteRequest turns a local Dockerfile build into a dispatchable
// one, re-rooting DockerfilePath relative to the context.
func NewRemoteRequest(req Request, cache CacheConfig) (RemoteRequest, error) {
	if err := req.Validate(); err != nil {
		return RemoteRequest{}, err
	}

	rel := ""
	if req.DockerfilePath != "" {
		var err error
		rel, err = filepath.Rel(req.ContextDir, req.dockerfilePath())
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return RemoteRequest{}, fmt.Errorf("%w: %q is not inside %q", ErrRemoteDockerfileOutsideContext, req.dockerfilePath(), req.ContextDir)
		}
	}

	return RemoteRequest{
		Kind:           RemoteKindDockerfile,
		ContextDir:     req.ContextDir,
		DockerfilePath: filepath.ToSlash(rel),
		Tag:            req.Tag,
		Target:         req.Target,
		BuildArgs:      req.BuildArgs,
		NoCache:        req.NoCache,
		Cache:          CacheConfig{RegistryRef: cache.RegistryRef, RegistryInsecure: cache.RegistryInsecure},
	}, nil
}

// NewRemoteRailpackRequest turns a local Railpack build into a
// dispatchable one.
func NewRemoteRailpackRequest(req RailpackRequest, cache CacheConfig) (RemoteRequest, error) {
	if err := req.Validate(); err != nil {
		return RemoteRequest{}, err
	}
	return RemoteRequest{
		Kind:       RemoteKindRailpack,
		ContextDir: req.SourceDir,
		Tag:        req.Tag,
		Cache:      CacheConfig{RegistryRef: cache.RegistryRef, RegistryInsecure: cache.RegistryInsecure},
	}, nil
}

// SolveRemote runs a build dispatched from a control plane against this
// node's own BuildKit and writes the resulting docker-save tar to out,
// rather than loading it into this node's image store: the image is
// wanted by whoever asked for the build, not by the node that ran it.
//
// req.ContextDir must already hold the unpacked build context.
func (c *Client) SolveRemote(ctx context.Context, req RemoteRequest, out io.Writer, progress func(ProgressEvent)) (*Result, error) {
	if progress == nil {
		progress = func(ProgressEvent) {}
	}

	switch req.Kind {
	case RemoteKindDockerfile:
		dockerfilePath := ""
		if req.DockerfilePath != "" {
			dockerfilePath = filepath.Join(req.ContextDir, filepath.FromSlash(req.DockerfilePath))
		}
		return c.solveDockerfile(ctx, Request{
			ContextDir:     req.ContextDir,
			DockerfilePath: dockerfilePath,
			Tag:            req.Tag,
			Target:         req.Target,
			BuildArgs:      req.BuildArgs,
			NoCache:        req.NoCache,
		}, req.Cache, out, progress)
	case RemoteKindRailpack:
		// Railpack ignores cache config here exactly as BuildRailpack does
		// locally: newRailpackSolveOpt wires no cache entries at all.
		return c.solveRailpack(ctx, RailpackRequest{SourceDir: req.ContextDir, Tag: req.Tag}, out, progress)
	default:
		return nil, fmt.Errorf("build: dispatched build has an unrecognized kind %q", req.Kind)
	}
}
