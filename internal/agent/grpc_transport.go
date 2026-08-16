package agent

// This file: GRPCTransport, the control-plane side of TASKS.md 3.2's
// reverse-dialed connection. An agent.Transport (TASKS.md 3.1)
// implemented by dispatching every docker.Runtime-shaped call over a
// real agent's Session stream via mux (mux.go), instead of Local's
// direct in-process call.

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// GRPCTransport implements Transport by dispatching every call over an
// established Session stream to a real, remote agent. Built from a
// *mux, not a raw stream: the request/response multiplexing this type
// depends on is already handled underneath it. Unexported constructor
// (newGRPCTransport): callers get one of these from the control plane's
// Session handler once an agent's stream is accepted and authenticated
// (server.go, TASKS.md 3.2's remaining wiring), never constructed
// directly.
type GRPCTransport struct {
	mux *mux
}

func newGRPCTransport(m *mux) *GRPCTransport {
	return &GRPCTransport{mux: m}
}

var _ Transport = (*GRPCTransport)(nil)

// InspectByName implements Transport (docker.Runtime).
func (t *GRPCTransport) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_InspectByName{InspectByName: &agentpb.InspectByNameRequest{Name: name}},
	})
	if err != nil {
		return nil, err
	}
	got := resp.GetInspectByName()
	if got == nil || !got.GetFound() {
		return nil, nil
	}
	return containerStateFromPB(got.GetState()), nil
}

// Create implements Transport (docker.Runtime).
func (t *GRPCTransport) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Create{Create: &agentpb.CreateRequest{Spec: containerSpecToPB(spec)}},
	})
	if err != nil {
		return "", err
	}
	return resp.GetCreate().GetId(), nil
}

// Start implements Transport (docker.Runtime).
func (t *GRPCTransport) Start(ctx context.Context, id string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: id}},
	})
	return err
}

// Stop implements Transport (docker.Runtime). timeout < 0 (wait
// indefinitely, docker.Runtime.Stop's own convention) is carried across
// the wire as TimeoutMs: -1, not clamped to 0, so the agent applies the
// identical "wait indefinitely" semantics rather than a silently
// different one.
func (t *GRPCTransport) Stop(ctx context.Context, id string, timeout time.Duration) error {
	ms := timeout.Milliseconds()
	if timeout < 0 {
		ms = -1
	}
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Stop{Stop: &agentpb.StopRequest{Id: id, TimeoutMs: ms}},
	})
	return err
}

// Remove implements Transport (docker.Runtime).
func (t *GRPCTransport) Remove(ctx context.Context, id string, force bool) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Remove{Remove: &agentpb.RemoveRequest{Id: id, Force: force}},
	})
	return err
}

// ListImages implements Transport (docker.Runtime).
func (t *GRPCTransport) ListImages(ctx context.Context, repo string) ([]docker.ImageInfo, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_ListImages{ListImages: &agentpb.ListImagesRequest{Repo: repo}},
	})
	if err != nil {
		return nil, err
	}
	return imageInfosFromPB(resp.GetListImages().GetImages()), nil
}

// ListByPrefix implements Transport (docker.Runtime).
func (t *GRPCTransport) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_ListByPrefix{ListByPrefix: &agentpb.ListByPrefixRequest{Prefix: prefix}},
	})
	if err != nil {
		return nil, err
	}
	return containerStatesFromPB(resp.GetListByPrefix().GetContainers()), nil
}

// Exec implements Transport (docker.Runtime). Deliberately not wired to
// the remote agent yet: this is a known, documented gap, not an
// oversight. docker.Runtime.Exec was added for internal/backup's Dumper,
// and every other Transport method here fits mux.Call's single
// request/response shape (or, for Events, its own explicit streaming
// workaround, see Events' doc comment above); Exec's ReadCloser return
// needs the same kind of dedicated streaming path Events required, plus
// a new AgentRequest/AgentResponse op in internal/agent/agentpb, which
// means a proto change and regeneration, not a small extension of the
// existing dispatch switch in execute.go. That is real, separate work.
// Rather than fake it with a call that would silently produce no data or
// the wrong data, a database backup for a service placed on a remote
// node fails loudly with this error today; only Local's in-process
// Transport (this control plane's own node) supports Exec until the
// wiring above lands.
func (t *GRPCTransport) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("agent: remote Exec not implemented over the agent transport (container %q, cmd %v): backups only support databases placed on this control plane's own node today", containerID, cmd)
}

// ExecWithInput implements Transport (docker.Runtime). Exec's own doc
// comment applies identically here, and then some: ExecWithInput needs
// everything Exec would (a dedicated streaming path, a new agentpb op)
// plus a second stream direction for stdin, so it inherits the same
// documented gap rather than a new one. A database restore for a service
// placed on a remote node fails loudly with this error today, the same
// "fail loudly, never fake it" posture Exec's own doc comment describes,
// until the wiring above lands; only Local's in-process Transport (this
// control plane's own node) supports ExecWithInput until then.
func (t *GRPCTransport) ExecWithInput(_ context.Context, containerID string, cmd []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, fmt.Errorf("agent: remote ExecWithInput not implemented over the agent transport (container %q, cmd %v): restores only support databases placed on this control plane's own node today", containerID, cmd)
}

// EnsureNetwork implements Transport (docker.Runtime). Deliberately not
// wired to the remote agent yet, the same documented gap Exec's own doc
// comment describes: a new agentpb op and proto regeneration, not a
// small extension of the existing dispatch switch. Per-app Docker
// networking (this method's own caller,
// internal/reconcile/application's Controller) is single-node scope
// today; a service placed on a remote node fails loudly here rather
// than silently reconciling without one.
func (t *GRPCTransport) EnsureNetwork(_ context.Context, name string) (string, error) {
	return "", fmt.Errorf("agent: remote EnsureNetwork not implemented over the agent transport (network %q): multi-service networking only supports services placed on this control plane's own node today", name)
}

// RemoveNetwork implements Transport (docker.Runtime). Same documented
// gap as EnsureNetwork above.
func (t *GRPCTransport) RemoveNetwork(_ context.Context, name string) error {
	return fmt.Errorf("agent: remote RemoveNetwork not implemented over the agent transport (network %q): multi-service networking only supports services placed on this control plane's own node today", name)
}

// ListNetworksByPrefix implements Transport (docker.Runtime). Same
// documented gap as EnsureNetwork above.
func (t *GRPCTransport) ListNetworksByPrefix(_ context.Context, prefix string) ([]docker.NetworkInfo, error) {
	return nil, fmt.Errorf("agent: remote ListNetworksByPrefix not implemented over the agent transport (prefix %q): multi-service networking only supports services placed on this control plane's own node today", prefix)
}

// EnsureVolume implements Transport (docker.Runtime).
func (t *GRPCTransport) EnsureVolume(ctx context.Context, name string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_EnsureVolume{EnsureVolume: &agentpb.EnsureVolumeRequest{Name: name}},
	})
	return err
}

// Events implements Transport (docker.Runtime). The one method that
// doesn't fit mux.Call's request/response shape: docker.Runtime.Events
// returns two channels streaming until ctx is cancelled, not one
// response. This issues a single WatchEvents request (acked immediately
// like any other Empty-result call, then relays every ProxiedEvent
// mux.subscribe's channel receives onto a fresh pair of channels this
// method returns, translating docker.Event's wire shape back and
// closing both channels once ctx is done or the subscription itself
// ends (session closed, agent-side stream error). ADR 003's push-not-
// poll event design, real end to end.
//
// Backpressure: mux's eventChanBuffer (64) bounds how far this can fall
// behind the agent's own event rate before frames start being dropped
// rather than blocking the shared recvLoop; a caller needing a
// guarantee stronger than "recent events, best effort" would need a
// different mechanism, not built here.
func (t *GRPCTransport) Events(ctx context.Context) (<-chan docker.Event, <-chan error) {
	events := make(chan docker.Event)
	errs := make(chan error, 1)

	watchID, err := randomRequestID() // same shape, a different logical ID space
	if err != nil {
		errs <- err
		close(events)
		close(errs)
		return events, errs
	}

	sub := t.mux.subscribe(watchID)
	if sub == nil {
		errs <- ErrSessionClosed
		close(events)
		close(errs)
		return events, errs
	}

	// The WatchEvents round trip itself happens inside the goroutine,
	// not before returning: docker.Client.Events (internal/docker)
	// returns its channels immediately and subscribes asynchronously,
	// and callers (internal/telemetry.LogCollector,
	// internal/reconcile.Engine.Run) rely on that non-blocking contract.
	// Blocking this call on a network round trip would be a real,
	// silent behavior change for exactly the same interface.
	go func() {
		defer close(events)
		defer close(errs)
		defer t.mux.unsubscribe(watchID)

		if _, callErr := t.mux.Call(ctx, &agentpb.AgentRequest{
			Op: &agentpb.AgentRequest_WatchEvents{WatchEvents: &agentpb.WatchEventsRequest{WatchId: watchID}},
		}); callErr != nil {
			errs <- callErr
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				events <- docker.Event{
					Action:        docker.EventAction(ev.GetAction()),
					ContainerName: ev.GetContainerName(),
					Time:          timestampFromPB(ev.GetTime()),
				}
			}
		}
	}()

	return events, errs
}
