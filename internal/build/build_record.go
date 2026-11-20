package build

import (
	"context"
	"fmt"
	"log/slog"
)

// RecordInput is what the control plane knows when a remote builder's digest is accepted.
type RecordInput struct {
	App          string
	Ref          string
	Tag          string
	Digest       string
	BuilderNode  string
	BuilderTrust string
}

// RecordSigner signs and stores a build record and returns its id.
type RecordSigner func(ctx context.Context, in RecordInput) (string, error)

// WithRouterRecorder turns on signed build records for registry-mode remote builds.
func WithRouterRecorder(f RecordSigner) RouterOption {
	return func(r *Router) { r.recorder = f }
}

// nodeTier is the builder tier of a node, server when it is unknown.
func (r *Router) nodeTier(ctx context.Context, nodeID string) string {
	if r.nodes == nil {
		return TrustServer
	}
	infos, err := r.nodes(ctx)
	if err != nil {
		return TrustServer
	}
	for _, n := range infos {
		if n.ID == nodeID {
			return n.tier()
		}
	}
	return TrustServer
}

// markBuilder stamps who built the image so the deploy path can apply tier policy.
func (r *Router) markBuilder(ctx context.Context, res *Result, nodeID string) {
	if res == nil {
		return
	}
	res.BuilderNodeID = nodeID
	res.BuilderTrust = r.nodeTier(ctx, nodeID)
}

// recordBuild signs the accepted digest. A personal builder without a record fails closed.
func (r *Router) recordBuild(ctx context.Context, nodeID, ref, tag string, res *Result) error {
	tier := r.nodeTier(ctx, nodeID)
	if r.recorder == nil {
		if tier == TrustPersonal {
			return fmt.Errorf("build: no signing key to record a personal builder result")
		}
		return nil
	}
	id, err := r.recorder(ctx, RecordInput{App: AppFromTag(tag), Ref: ref, Tag: tag, Digest: res.Digest, BuilderNode: nodeID, BuilderTrust: tier})
	if err != nil {
		if tier == TrustPersonal {
			return fmt.Errorf("build: record personal builder result: %w", err)
		}
		r.logger.Warn("build: signed record skipped", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return nil
	}
	res.RecordID = id
	return nil
}
