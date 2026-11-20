package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// TransferConfig is what registry transfer needs; any missing piece means tar.
type TransferConfig struct {
	Mode TransferMode
	// RegistryHost is the control plane registry host the builder can reach.
	RegistryHost string
	Insecure     bool
	// TokenAuth is true only when the registry is confirmed to run in token-auth mode.
	TokenAuth bool
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
	return c.Mode == TransferRegistry && c.TokenAuth && c.RegistryHost != "" && c.TokenTTL > 0 &&
		c.Issuer != nil && c.Verifier != nil && c.Pull != nil
}

// dispatchWith picks the transfer path for one dispatched build.
func (r *Router) dispatchWith(ctx context.Context, nodeID string, req RemoteRequest, progress func(ProgressEvent)) (*Result, error) {
	res, err := r.dispatchTransfer(ctx, nodeID, req, progress)
	r.markBuilder(ctx, res, nodeID)
	return res, err
}

func (r *Router) dispatchTransfer(ctx context.Context, nodeID string, req RemoteRequest, progress func(ProgressEvent)) (*Result, error) {
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
	res, err := r.dispatchRegistry(ctx, nodeID, req, cfg, progress)
	var pe *pushAuthError
	if err != nil && errors.As(err, &pe) && ctx.Err() == nil {
		r.logger.Warn("build: registry push was refused, using tar", slog.String("node_id", nodeID))
		return r.dispatch(ctx, nodeID, req, progress)
	}
	return res, err
}

// pushAuthError marks a build failure that came from the registry refusing the push.
type pushAuthError struct{ err error }

func (e *pushAuthError) Error() string { return e.err.Error() }
func (e *pushAuthError) Unwrap() error { return e.err }

func isPushAuthFailure(err error) bool {
	m := strings.ToLower(err.Error())
	for _, h := range []string{"unauthorized", "401", "insufficient_scope", "authentication required", "failed to fetch oauth token", "failed to fetch anonymous token", "denied: requested access"} {
		if strings.Contains(m, h) {
			return true
		}
	}
	return false
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
	token, err := cfg.Issuer.IssueWithCache(repo, tag, cacheRepoFull(req.Cache, cfg.RegistryHost), cfg.TokenTTL)
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
		wrapped := fmt.Errorf("build: on node %s: %w", nodeID, err)
		if isPushAuthFailure(err) {
			return nil, &pushAuthError{err: wrapped}
		}
		return nil, wrapped
	}
	if err := VerifyPushedDigest(ctx, cfg.Verifier, ref, res.Digest); err != nil {
		return nil, err
	}
	if err := r.recordBuild(ctx, nodeID, ref, req.Tag, res); err != nil {
		return nil, err
	}
	// Pull is by digest only; the registry tag is never trusted as the image.
	if err := cfg.Pull(ctx, ref, res.Digest, req.Tag); err != nil {
		return nil, fmt.Errorf("build: pull pushed image: %w", err)
	}
	return res, nil
}

// cacheRepoFull is the host-qualified cache repository when the cache lives on the control plane registry.
func cacheRepoFull(cache CacheConfig, host string) string {
	p := cacheRepoPath(cache, host)
	if p == "" {
		return ""
	}
	return host + "/" + p
}

// localCacheAuth mints a cache-only token for a control plane build, or nil when the cache is not on the token registry.
func (r *Router) localCacheAuth(ctx context.Context, req Request) (*CacheAuth, func()) {
	if r.transfer == nil {
		return nil, func() {}
	}
	cfg := r.transfer(ctx)
	if cfg.Mode != TransferRegistry || !cfg.TokenAuth || cfg.Issuer == nil || cfg.TokenTTL <= 0 || cfg.RegistryHost == "" {
		return nil, func() {}
	}
	repo := cacheRepoFull(r.local.cache.ForApp(req.cacheID()), cfg.RegistryHost)
	if repo == "" {
		return nil, func() {}
	}
	tok, err := cfg.Issuer.Issue(repo, "cache", cfg.TokenTTL)
	if err != nil {
		return nil, func() {}
	}
	return &CacheAuth{Token: tok}, func() { cfg.Issuer.Revoke(tok) }
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
