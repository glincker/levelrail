package build

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

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

	// policy, query, loads and slotLimit feed ChooseBuildNode; all optional,
	// and with none set the choice is the original first online node.
	policy    func(ctx context.Context) Policy
	query     func(ctx context.Context, app string) Query
	loads     func() []NodeLoad
	slotLimit func(ctx context.Context, app string) int

	// load takes the image tar a dispatched build streamed back and puts
	// it in this control plane's own image store. A field rather than a
	// direct call so the dispatch path can be exercised without a live
	// Docker daemon.
	load func(ctx context.Context, r io.Reader, tag string, progress func(ProgressEvent)) error

	// transfer is the live registry transfer config; nil keeps the tar path.
	transfer func(ctx context.Context) TransferConfig
}

// RouterOption configures optional Router behavior.
type RouterOption func(*Router)

// WithRouterLogger overrides the logger a dispatch decision is reported
// on. Defaults to slog.Default().
func WithRouterLogger(logger *slog.Logger) RouterOption {
	return func(r *Router) { r.logger = logger }
}

// WithRouterPolicy sets the live routing policy read on every build.
func WithRouterPolicy(f func(ctx context.Context) Policy) RouterOption {
	return func(r *Router) { r.policy = f }
}

// WithRouterQuery fills the per-app part of a placement: target architecture, labels, pinned or preferred runner.
func WithRouterQuery(f func(ctx context.Context, app string) Query) RouterOption {
	return func(r *Router) { r.query = f }
}

// WithRouterLoad supplies per-node slot usage, normally Limiter.Load.
func WithRouterLoad(f func() []NodeLoad) RouterOption {
	return func(r *Router) { r.loads = f }
}

// WithRouterSlotLimit supplies the slot limit assumed for a node that has not built yet.
func WithRouterSlotLimit(f func(ctx context.Context, app string) int) RouterOption {
	return func(r *Router) { r.slotLimit = f }
}

// WithRouterTransfer enables registry transfer: f is read on every build, so
// flipping build.transfer_mode applies live. Incomplete config falls back to tar.
func WithRouterTransfer(f func(ctx context.Context) TransferConfig) RouterOption {
	return func(r *Router) { r.transfer = f }
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
	nodeID, err := r.selectNode(ctx, AppFromTag(req.Tag))
	if err != nil {
		return nil, err
	}
	if nodeID == "" {
		return r.local.Build(ctx, req, progress)
	}

	if len(req.Secrets) > 0 {
		return nil, ErrRemoteSecretsUnsupported
	}
	if req.S3Cache != nil {
		emitCacheWarning(progress, "build cache skipped: this build runs on a remote build node, which does not receive bucket credentials")
	}
	if req.Network, err = ResolveForDispatch(ctx); err != nil {
		return nil, err
	}
	remoteReq, err := NewRemoteRequest(req, r.local.cache)
	if err != nil {
		return nil, err
	}
	return r.dispatchWith(ctx, nodeID, remoteReq, progress)
}

// BuildRailpack implements internal/deploy.ImageBuilder.
func (r *Router) BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error) {
	nodeID, err := r.selectNode(ctx, AppFromTag(req.Tag))
	if err != nil {
		return nil, err
	}
	if nodeID == "" {
		return r.local.BuildRailpack(ctx, req, progress)
	}

	if req.Network, err = ResolveForDispatch(ctx); err != nil {
		return nil, err
	}
	remoteReq, err := NewRemoteRailpackRequest(req, r.local.cache)
	if err != nil {
		return nil, err
	}
	return r.dispatchWith(ctx, nodeID, remoteReq, progress)
}

// SelectedNode names the node the next build would run on, "" for the local one.
func (r *Router) SelectedNode(ctx context.Context) string {
	return r.Decide(ctx, "").Node
}

// Decide places a build of app and says why. A failure to list nodes means the
// control plane, never a failed build.
func (r *Router) Decide(ctx context.Context, app string) Decision {
	d, err := r.decide(ctx, app)
	if err != nil {
		return Decision{Reason: "control plane: could not list build nodes"}
	}
	return d
}

// ImageSize reports a built image's size in bytes from the local image store, 0 when unknown.
func (r *Router) ImageSize(ctx context.Context, tag string) int64 {
	info, err := r.local.docker.ImageInspect(ctx, tag)
	if err != nil {
		return 0
	}
	return info.Size
}

// selectNode honors a decision the Limiter already made for this build, so the
// slot it queued on and the node it dispatches to cannot disagree.
func (r *Router) selectNode(ctx context.Context, app string) (string, error) {
	if d, ok := DecisionFrom(ctx); ok {
		return d.Node, nil
	}
	d, err := r.decide(ctx, app)
	return d.Node, err
}

func (r *Router) decide(ctx context.Context, app string) (Decision, error) {
	if r.nodes == nil || r.remote == nil {
		return Decision{Reason: "control plane: no build nodes configured"}, nil
	}
	nodes, err := r.nodes(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("build: list build nodes: %w", err)
	}
	policy := r.policy
	if policy == nil {
		policy = currentRunnerPolicy()
	}
	if policy == nil {
		id := SelectBuildNode(nodes)
		if id == "" {
			return Decision{Reason: "control plane: no build node is online and accepting work"}, nil
		}
		return Decision{Node: id, Reason: "first online build node"}, nil
	}
	if r.loads != nil {
		nodes = ApplyLoad(nodes, r.loads(), r.defaultSlots(ctx, app))
	}
	var q Query
	if r.query != nil {
		q = r.query(ctx, app)
	}
	q.App = app
	p := policy(ctx)
	if p.Now.IsZero() {
		p.Now = time.Now()
	}
	return ChooseBuildNode(nodes, q, p), nil
}

func (r *Router) defaultSlots(ctx context.Context, app string) int {
	if r.slotLimit == nil {
		return 0
	}
	return r.slotLimit(ctx, app)
}

type decisionKey struct{}

// WithDecision carries a placement through the build call chain.
func WithDecision(ctx context.Context, d Decision) context.Context {
	return context.WithValue(ctx, decisionKey{}, d)
}

// DecisionFrom returns the placement already made for this build, if any.
func DecisionFrom(ctx context.Context) (Decision, bool) {
	d, ok := ctx.Value(decisionKey{}).(Decision)
	return d, ok
}

// ApplyLoad copies slot usage onto nodes; a node without usage gets defaultTotal slots, 0 meaning unknown.
func ApplyLoad(nodes []NodeInfo, loads []NodeLoad, defaultTotal int) []NodeInfo {
	byNode := make(map[string]NodeLoad, len(loads))
	for _, l := range loads {
		byNode[l.Node] = l
	}
	out := make([]NodeInfo, len(nodes))
	copy(out, nodes)
	for i := range out {
		l, ok := byNode[out[i].ID]
		out[i].SlotsTotal = defaultTotal
		if ok {
			out[i].SlotsUsed, out[i].Waiting = l.Running, l.Waiting
			if l.Limit > 0 {
				out[i].SlotsTotal = l.Limit
			}
		}
	}
	return out
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
