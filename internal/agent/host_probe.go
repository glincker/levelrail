package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/kit/probe"
)

// CapHostProbe means the agent runs readiness probes on its own host. A node
// with no mesh route is probed through it, since the control plane cannot
// dial the node's published ports.
const CapHostProbe = "host_probe"

// maxProbeTimeoutFactor caps a requested timeout at this multiple of the probe default.
const maxProbeTimeoutFactor = 10

// errProbeNotLoopback keeps the agent from being used to dial arbitrary hosts.
var errProbeNotLoopback = errors.New("agent: host probe only dials a loopback address")

// executeProbe runs one attempt against a loopback port of this host.
func executeProbe(ctx context.Context, req *agentpb.ProbeRequest) (*agentpb.ProbeResponse, error) {
	host, _, err := net.SplitHostPort(req.GetAddr())
	if err != nil {
		return nil, fmt.Errorf("agent: host probe address %q: %w", req.GetAddr(), err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, errProbeNotLoopback
	}
	timeout := time.Duration(req.GetTimeoutNs())
	if maxTimeout := maxProbeTimeoutFactor * probe.DefaultTimeout; timeout > maxTimeout {
		timeout = maxTimeout
	}
	cfg := probe.Config{
		TCP:            req.GetTcp(),
		Path:           req.GetPath(),
		Scheme:         req.GetScheme(),
		Host:           req.GetHost(),
		TLSSkipVerify:  req.GetTlsSkipVerify(),
		ExpectedStatus: req.GetExpectedStatus(),
		Timeout:        timeout,
	}
	if req.GetHasFollowRedirects() {
		follow := req.GetFollowRedirects()
		cfg.FollowRedirects = &follow
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("agent: host probe config: %w", err)
	}
	out := &agentpb.ProbeResponse{}
	start := time.Now()
	perr := probe.New(nil, nil, probe.Limits{MaxRedirects: int(req.GetMaxRedirects()), DefaultTimeout: timeout}, probe.WithLoopbackOnly()).Check(ctx, probe.Target{Addr: req.GetAddr()}, cfg)
	if perr != nil {
		out.FailureReason = perr.Error()
		var f *probe.Failure
		if errors.As(perr, &f) {
			out.FailureKind = string(f.Kind)
		}
	}
	logProbeAttempt(ctx, cfg.TCP, req.GetAddr(), out.GetFailureKind(), perr != nil, time.Since(start))
	return out, nil
}

// logProbeAttempt records one attempt for debug sessions; it never includes bodies or headers.
func logProbeAttempt(ctx context.Context, tcp bool, addr, failureKind string, failed bool, elapsed time.Duration) {
	kind, outcome := "http", "ok"
	if tcp {
		kind = "tcp"
	}
	if failed {
		outcome = failureKind
		if outcome == "" {
			outcome = "failed"
		}
	}
	slog.DebugContext(ctx, "host probe attempt",
		slog.String("kind", kind), slog.String("target", addr),
		slog.String("outcome", outcome), slog.Int64("elapsed_ms", elapsed.Milliseconds()))
}

// ProbesOnHost reports whether this node's agent can run probes itself.
func (t *GRPCTransport) ProbesOnHost() bool { return t.caps.has(CapHostProbe) }

// CheckRemote implements probe.RemoteChecker by asking the agent to probe addr from its host.
func (t *GRPCTransport) CheckRemote(ctx context.Context, addr string, cfg probe.Config) error {
	if !t.ProbesOnHost() {
		return fmt.Errorf("%w: %w", probe.ErrRemoteUnsupported, ErrAgentLacksCapability)
	}
	req := &agentpb.ProbeRequest{
		Addr: addr, Tcp: cfg.TCP, Path: cfg.Path, Scheme: cfg.Scheme, Host: cfg.Host,
		TlsSkipVerify: cfg.TLSSkipVerify, ExpectedStatus: cfg.ExpectedStatus, TimeoutNs: int64(cfg.Timeout),
		MaxRedirects: int32(cfg.MaxRedirects), //nolint:gosec // bounded by the operator's redirect limit
	}
	if cfg.FollowRedirects != nil {
		req.HasFollowRedirects, req.FollowRedirects = true, *cfg.FollowRedirects
	}
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Probe{Probe: req}})
	if err != nil {
		return fmt.Errorf("agent: host probe: %w", err)
	}
	got := resp.GetProbe()
	if got == nil {
		return errors.New("agent: host probe: empty response")
	}
	if got.GetFailureReason() == "" {
		return nil
	}
	return &probe.Failure{Kind: probe.FailureKind(got.GetFailureKind()), Reason: got.GetFailureReason()}
}

var _ probe.RemoteChecker = (*GRPCTransport)(nil)
