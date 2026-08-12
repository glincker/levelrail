package build

import (
	"fmt"
	"io"
	"path/filepath"

	bkclient "github.com/moby/buildkit/client"
	"github.com/tonistiigi/fsutil"
)

// newSolveOpt turns a Request into the SolveOpt BuildKit's client needs.
// It uses the dockerfile.v0 frontend so any plain Dockerfile is accepted
// (CLAUDE.md 4.9), and an exporter of type "docker", which produces a
// `docker save`-style tar stream on out. The caller is responsible for
// loading that stream into the local image store via the Docker Engine
// API's /images/load endpoint (see loadImage in build.go); this keeps the
// whole path CLI-free, in contrast to BuildKit's own
// build-using-dockerfile example, which shells out to `docker load`.
//
// cacheDir, when non-empty, wires BuildKit's "local" cache backend as
// both import and export: every build tries to reuse whatever the
// previous build left in cacheDir, then updates it. Empty disables
// caching for this build, not an empty-but-present cache.
func newSolveOpt(req Request, cacheDir string, out io.WriteCloser) (*bkclient.SolveOpt, error) {
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

	if cacheDir != "" {
		opt.CacheImports = []bkclient.CacheOptionsEntry{
			{Type: "local", Attrs: map[string]string{"src": cacheDir}},
		}
		opt.CacheExports = []bkclient.CacheOptionsEntry{
			{Type: "local", Attrs: map[string]string{"dest": cacheDir, "mode": "max"}},
		}
	}

	return opt, nil
}
