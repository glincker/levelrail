package agent

// GRPCSink is the control-plane side of mesh config distribution to remote agents.
// It resolves each node's registered transport and forwards the call over that node's connection.
import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

// meshTransport is the slice of *GRPCTransport GRPCSink needs, as an interface so tests can fake it.
type meshTransport interface {
	ApplyMesh(ctx context.Context, cfg network.DeviceConfig) (network.NodeIdentity, error)
	RotateMeshKey(ctx context.Context) (network.RotationResult, error)
}

// GRPCSink dispatches mesh config to remote nodes over their own agent
// Session stream, resolved per call through registry.
type GRPCSink struct {
	registry *Registry
	policy   ApplyPolicy
}

// ApplyPolicy bounds and retries GRPCSink.ApplyMesh. Each func is read per
// call so a runtime settings change applies on the next pass; nil means unbounded.
type ApplyPolicy struct {
	// Timeout bounds one attempt; zero or less leaves it to the caller's context.
	Timeout func() time.Duration
	// Retries is how many extra attempts a dropped session gets.
	Retries func() int
	// Backoff is the first retry delay, doubled per retry and capped at Timeout.
	Backoff func() time.Duration
	// Sleep waits between attempts; tests swap it for a fake clock.
	Sleep func(ctx context.Context, d time.Duration) error
}

// SinkOption configures NewGRPCSink.
type SinkOption func(*GRPCSink)

// WithApplyPolicy sets the per-attempt timeout and retry policy for ApplyMesh.
func WithApplyPolicy(p ApplyPolicy) SinkOption {
	return func(s *GRPCSink) { s.policy = p }
}

// NewGRPCSink builds a GRPCSink that resolves node transports through registry.
func NewGRPCSink(registry *Registry, opts ...SinkOption) *GRPCSink {
	s := &GRPCSink{registry: registry}
	for _, o := range opts {
		o(s)
	}
	return s
}

var (
	_ network.ConfigSink = (*GRPCSink)(nil)
	_ network.KeyRotator = (*GRPCSink)(nil)
)

// ErrNodeNotMeshCapable is returned when a node's registered transport does not support mesh operations. Real agent connections never hit it.
var ErrNodeNotMeshCapable = fmt.Errorf("network: node's transport does not support mesh operations")

// ApplyMesh forwards cfg to nodeID's live agent session, failing only that node's pass if it has no registered transport.
// A closed session (a reconnect race) is retried with backoff; a timed out attempt is not.
func (s *GRPCSink) ApplyMesh(ctx context.Context, nodeID string, cfg network.DeviceConfig) (network.NodeIdentity, error) {
	retries := 0
	if s.policy.Retries != nil {
		retries = max(s.policy.Retries(), 0)
	}
	delay := durationOr(s.policy.Backoff)
	timeout := durationOr(s.policy.Timeout)
	sleep := s.policy.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	for attempt := 0; ; attempt++ {
		id, err := s.applyOnce(ctx, nodeID, cfg, timeout)
		if err == nil || attempt >= retries || !transientApplyErr(ctx, err) {
			return id, err
		}
		if serr := sleep(ctx, delay); serr != nil {
			return network.NodeIdentity{}, err
		}
		delay *= 2
		if timeout > 0 && delay > timeout {
			delay = timeout
		}
	}
}

// applyOnce re-resolves the transport every attempt so a reconnected node's new session is used.
func (s *GRPCSink) applyOnce(ctx context.Context, nodeID string, cfg network.DeviceConfig, timeout time.Duration) (network.NodeIdentity, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	t, err := s.registry.Get(nodeID)
	if err != nil {
		return network.NodeIdentity{}, err
	}
	mt, ok := t.(meshTransport)
	if !ok {
		return network.NodeIdentity{}, fmt.Errorf("%w: node %q", ErrNodeNotMeshCapable, nodeID)
	}
	id, err := mt.ApplyMesh(ctx, cfg)
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return id, fmt.Errorf("apply mesh on node %q timed out after %s: %w", nodeID, timeout, err)
	}
	return id, err
}

// transientApplyErr retries only a dropped session: a timed out push may still land on the
// agent, so resending it could overwrite a newer pass's config with a stale one.
func transientApplyErr(parent context.Context, err error) bool {
	if parent.Err() != nil {
		return false
	}
	return errors.Is(err, ErrSessionClosed)
}

func durationOr(f func() time.Duration) time.Duration {
	if f == nil {
		return 0
	}
	return max(f(), 0)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RotateKey implements network.KeyRotator by asking nodeID's live agent
// Session to rotate its own key. Same not-registered/not-mesh-capable
// failure modes as ApplyMesh.
func (s *GRPCSink) RotateKey(ctx context.Context, nodeID string) (network.RotationResult, error) {
	t, err := s.registry.Get(nodeID)
	if err != nil {
		return network.RotationResult{}, err
	}
	mt, ok := t.(meshTransport)
	if !ok {
		return network.RotationResult{}, fmt.Errorf("%w: node %q", ErrNodeNotMeshCapable, nodeID)
	}
	result, err := mt.RotateMeshKey(ctx)
	if err != nil {
		return network.RotationResult{}, err
	}
	result.NodeID = nodeID
	return result, nil
}
