package build

import (
	"fmt"
	"io"
	"path/filepath"

	bkclient "github.com/moby/buildkit/client"
	"github.com/tonistiigi/fsutil"
)

// newSolveOpt turns a Request into the SolveOpt BuildKit's client needs.
// It uses the dockerfile.v0 frontend so any plain Dockerfile is accepted,
// per the declarative app spec's build config, and an exporter of type
// "docker", which produces a
// `docker save`-style tar stream on out. The caller is responsible for
// loading that stream into the local image store via the Docker Engine
// API's /images/load endpoint (see loadImage in build.go); this keeps the
// whole path CLI-free, in contrast to BuildKit's own
// build-using-dockerfile example, which shells out to `docker load`.
//
// cache, when non-empty (CacheConfig.empty), wires every backend it
// configures as both import and export: every build tries to reuse
// whatever the previous build left in each configured backend, then
// updates it. A zero-value cache disables caching for this build
// entirely, not an empty-but-present cache.
func newSolveOpt(req Request, cache CacheConfig, out io.WriteCloser) (*bkclient.SolveOpt, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	dockerfilePath := req.dockerfilePath()

	contextFS, err := fsutil.NewFS(req.ContextDir)
	if err != nil {
		return nil, fmt.Errorf("build: invalid context dir %q: %w", req.ContextDir, err)
	}
	dockerfileFS, err := fsutil.NewFS(filepath.Dir(dockerfilePath))
	if err != nil {
		return nil, fmt.Errorf("build: invalid dockerfile dir for %q: %w", dockerfilePath, err)
	}

	attrs := map[string]string{
		"filename": filepath.Base(dockerfilePath),
	}
	if req.Target != "" {
		attrs["target"] = req.Target
	}
	if req.NoCache {
		attrs["no-cache"] = ""
	}
	for k, v := range req.BuildArgs {
		attrs["build-arg:"+k] = v
	}

	opt := &bkclient.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: attrs,
		LocalMounts: map[string]fsutil.FS{
			"context":    contextFS,
			"dockerfile": dockerfileFS,
		},
		Exports: []bkclient.ExportEntry{
			{
				Type: bkclient.ExporterDocker,
				Attrs: map[string]string{
					"name": req.Tag,
				},
				Output: func(map[string]string) (io.WriteCloser, error) {
					return out, nil
				},
			},
		},
	}

	// cache.entries() is called unconditionally, not gated on
	// cache.empty(): an inconsistent config (e.g. RegistryInsecure set
	// with no RegistryRef) must fail loudly here even though it has no
	// Dir or RegistryRef of its own to build entries from, rather than
	// silently building without caching because "empty" and "invalid"
	// got conflated.
	imports, exports, err := cache.entries()
	if err != nil {
		return nil, fmt.Errorf("build: cache config: %w", err)
	}
	opt.CacheImports = imports
	opt.CacheExports = exports

	return opt, nil
}
