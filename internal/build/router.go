package build

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"golang.org/x/sync/errgroup"
)

// NodeSource reports every node the control plane currently knows about,
// with whichever of them are reachable right now marked Online. Consulted
// per build rather than once at startup, so marking a node build-capable,
// or a build node dropping offline, takes effect on the next build
// instead of the next control-plane restart.
type NodeSource func(ctx context.Context) ([]NodeInfo, error)

// NodeBuilder runs a build on one specific node and streams the result
// back: progress as it happens, and the docker-save image tar into image.
// internal/agent's BuildDispatcher satisfies this over the agent
// transport; nothing in this package knows how a node is reached.
type NodeBuilder interface {
	BuildOnNode(ctx context.Context, nodeID string, req RemoteRequest, image io.Writer, progress func(ProgressEvent)) (*Result, error)
}

// Router is the build entry point the deploy pipeline calls: it picks a
// build node (SelectBuildNode, node.go) and either builds locally or
// dispatches, with the same method set and the same results either way.
type Router struct {
	local  *Client
	nodes  NodeSource
	remote NodeBuilder
	logger *slog.Logger

	// load takes the image tar a dispatched build streamed back and puts
	// it in this control plane's own image store. A field rather than a
	// direct call so the dispatch path can be exercised without a live
	// Docker daemon.
	load func(ctx context.Context, r io.Reader, tag string, progress func(ProgressEvent)) error
}

// RouterOption configures optional Router behavior.
type RouterOption func(*Router)

// WithRouterLogger overrides the logger a dispatch decision is reported
// on. Defaults to slog.Default().
func WithRouterLogger(logger *slog.Logger) RouterOption {
	return func(r *Router) { r.logger = logger }
}

// NewRouter builds a Router over local. nodes and remote may both be nil,
// which pins every build to local: that is the single-node default, not a
// degraded mode.
func NewRouter(local *Client, nodes NodeSource, remote NodeBuilder, opts ...RouterOption) *Router {
	r := &Router{local: local, nodes: nodes, remote: remote, logger: slog.Default()}
	r.load = func(ctx context.Context, tar io.Reader, tag string, progress func(ProgressEvent)) error {
		return loadImage(ctx, local.docker, tar, tag, progress)
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Build implements internal/deploy.ImageBuilder.
func (r *Router) Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error) {
	nodeID, err := r.selectNode(ctx)
	if err != nil {
		return nil, err
	}
	if nodeID == "" {
		return r.local.Build(ctx, req, progress)
	}

	remoteReq, err := NewRemoteRequest(req, r.local.cache)
	if err != nil {
		return nil, err
	}
	return r.dispatch(ctx, nodeID, remoteReq, progress)
}

// BuildRailpack implements internal/deploy.ImageBuilder.
func (r *Router) BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error) {
	nodeID, err := r.selectNode(ctx)
	if err != nil {
		return nil, err
	}
	if nodeID == "" {
		return r.local.BuildRailpack(ctx, req, progress)
	}

	remoteReq, err := NewRemoteRailpackRequest(req, r.local.cache)
	if err != nil {
		return nil, err
	}
	return r.dispatch(ctx, nodeID, remoteReq, progress)
}

func (r *Router) selectNode(ctx context.Context) (string, error) {
	if r.nodes == nil || r.remote == nil {
		return "", nil
	}
	nodes, err := r.nodes(ctx)
	if err != nil {
		return "", fmt.Errorf("build: list build nodes: %w", err)
	}
	return SelectBuildNode(nodes)
}

// dispatch runs req on nodeID and loads the image it streams back into
// this control plane's own image store, so a dispatched build leaves the
// platform in exactly the state a local one would have: only the CPU
// moved, not the result.
func (r *Router) dispatch(ctx context.Context, nodeID string, req RemoteRequest, progress func(ProgressEvent)) (*Result, error) {
	if progress == nil {
		progress = func(ProgressEvent) {}
	}
	r.logger.Info("build: dispatching to build node",
		slog.String("node_id", nodeID), slog.String("tag", req.Tag), slog.String("kind", string(req.Kind)))

	pipeR, pipeW := io.Pipe()
	eg, egCtx := errgroup.WithContext(ctx)

	var (
		res      *Result
		buildErr error
	)
	eg.Go(func() error {
		res, buildErr = r.remote.BuildOnNode(egCtx, nodeID, req, pipeW, progress)
		_ = pipeW.CloseWithError(buildErr)
		return buildErr
	})
	eg.Go(func() error {
		defer func() { _ = pipeR.Close() }()
		return r.load(egCtx, pipeR, req.Tag, progress)
	})

	err := eg.Wait()
	// The remote build's own error wins over the local image load's: a
	// build that never finished always breaks the load too, and only the
	// remote error says why it failed.
	if buildErr != nil {
		return nil, fmt.Errorf("build: on node %s: %w", nodeID, buildErr)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}
