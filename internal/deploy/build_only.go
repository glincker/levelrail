package deploy

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/build"
)

// BuildOnlyRequest builds one image without touching desired state.
type BuildOnlyRequest struct {
	// Type is "dockerfile" (default) or "railpack".
	Type          string
	SourceDir     string
	BaseDirectory string
	Dockerfile    string
	Tag           string
	Args          map[string]string
}

// BuildOnly runs the same builder deploys use and returns the built tag,
// leaving desired state alone so a pipeline can build, test, and deploy as
// separate steps.
func (p *Pipeline) BuildOnly(ctx context.Context, req BuildOnlyRequest, progress func(build.ProgressEvent)) (string, error) {
	root, err := resolveBuildRoot(req.SourceDir, req.BaseDirectory)
	if err != nil {
		return "", fmt.Errorf("deploy: build only: %w", err)
	}
	var res *build.Result
	switch req.Type {
	case "", "dockerfile":
		dockerfile := ""
		if req.Dockerfile != "" {
			dockerfile = filepath.Join(root, req.Dockerfile)
		}
		res, err = p.builder.Build(ctx, build.Request{ContextDir: root, DockerfilePath: dockerfile, Tag: req.Tag, BuildArgs: req.Args}, progress)
	case "railpack":
		res, err = p.builder.BuildRailpack(ctx, build.RailpackRequest{SourceDir: root, Tag: req.Tag}, progress)
		var unsupported *build.UnsupportedProviderError
		if errors.As(err, &unsupported) {
			return "", fmt.Errorf("deploy: build only: railpack detected unsupported provider %q", unsupported.Provider)
		}
	default:
		return "", fmt.Errorf("deploy: build only: unknown build type %q", req.Type)
	}
	if err != nil {
		return "", fmt.Errorf("deploy: build only: %w", err)
	}
	return res.Tag, nil
}
