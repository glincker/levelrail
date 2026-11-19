package build

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// TransferConfig is what registry transfer needs; any missing piece means tar.
type TransferConfig struct {
	Mode TransferMode
	// RegistryHost is the control plane registry host the builder can reach.
	RegistryHost string
	Insecure     bool
	// TokenTTL should cover the build timeout plus slack; the token dies with the job.
	TokenTTL time.Duration
	Issuer   *PushTokenIssuer
	Verifier DigestVerifier
	// Pull makes ref@digest available to the control plane's own image store under tag.
	Pull func(ctx context.Context, ref, digest, tag string) error
}

// PushCapable is implemented by a NodeBuilder that can tell whether a node's agent honours registry push.
type PushCapable interface {
	NodeSupportsBuildPush(nodeID string) bool
}

func (c TransferConfig) ready() bool {
	return c.Mode == TransferRegistry && c.RegistryHost != "" && c.TokenTTL > 0 &&
		c.Issuer != nil && c.Verifier != nil && c.Pull != nil
}

// dispatchWith picks the transfer path for one dispatched build.
func (r *Router) dispatchWith(ctx context.Context, nodeID string, req RemoteRequest, progress func(ProgressEvent)) (*Result, error) {
	if r.transfer == nil {
		return r.dispatch(ctx, nodeID, req, progress)
	}
	cfg := r.transfer(ctx)
	if !cfg.ready() {
		if cfg.Mode == TransferRegistry {
			r.logger.Warn("build: registry transfer is not fully configured, using tar", slog.String("node_id", nodeID))
		}
		return r.dispatch(ctx, nodeID, req, progress)
	}
	if pc, ok := r.remote.(PushCapable); !ok || !pc.NodeSupportsBuildPush(nodeID) {
		return r.dispatch(ctx, nodeID, req, progress)
	}
	return r.dispatchRegistry(ctx, nodeID, req, cfg, progress)
}

// dispatchRegistry has the builder push by digest, then trusts the digest only
// after the registry confirms it holds that manifest.
func (r *Router) dispatchRegistry(ctx context.Context, nodeID string, req RemoteRequest, cfg TransferConfig, progress func(ProgressEvent)) (*Result, error) {
	if progress == nil {
		progress = func(ProgressEvent) {}
	}
	ref := cfg.RegistryHost + "/" + req.Tag
	repo, tag, err := SplitRef(ref)
	if err != nil {
		return nil, err
	}
	token, err := cfg.Issuer.Issue(repo, tag, cfg.TokenTTL)
	if err != nil {
		return nil, err
	}
	// The token dies with the job, success or not.
	defer cfg.Issuer.Revoke(token)
	req.Push = &PushGrant{Ref: ref, Token: token, Insecure: cfg.Insecure}
	r.logger.Info("build: dispatching to build node with registry push",
		slog.String("node_id", nodeID), slog.Any("push", *req.Push))

	// No image frames come back in this mode, so the writer is a discard.
	res, err := r.remote.BuildOnNode(ctx, nodeID, req, discardWriter{}, progress)
	if err != nil {
		return nil, fmt.Errorf("build: on node %s: %w", nodeID, err)
	}
	if err := VerifyPushedDigest(ctx, cfg.Verifier, ref, res.Digest); err != nil {
		return nil, err
	}
	// Pull is by digest only; the registry tag is never trusted as the image.
	if err := cfg.Pull(ctx, ref, res.Digest, req.Tag); err != nil {
		return nil, fmt.Errorf("build: pull pushed image: %w", err)
	}
	return res, nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
