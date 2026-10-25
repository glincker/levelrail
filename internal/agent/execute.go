package agent

// Pure dispatch over docker.Runtime with no gRPC, so it is testable with a fake runtime.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// Execute returns exactly one AgentResponse per request, carrying req.RequestId, even on failure.
// Errors travel as plain strings because the gRPC boundary cannot carry %w wrapping.
func Execute(ctx context.Context, rt docker.Runtime, req *agentpb.AgentRequest, emitEvent func(*agentpb.ProxiedEvent)) *agentpb.AgentResponse {
	resp := &agentpb.AgentResponse{RequestId: req.GetRequestId()}

	switch op := req.GetOp().(type) {
	case *agentpb.AgentRequest_InspectByName:
		state, err := rt.InspectByName(ctx, op.InspectByName.GetName())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_InspectByName{InspectByName: &agentpb.InspectByNameResponse{
			Found: state != nil,
			State: containerStateToPB(state),
		}}

	case *agentpb.AgentRequest_InspectExitState:
		inspector, ok := rt.(docker.ExitStateInspector)
		if !ok {
			resp.Error = ErrExitStateUnsupported.Error()
			return resp
		}
		state, err := inspector.InspectExitState(ctx, op.InspectExitState.GetName())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_InspectExitState{InspectExitState: &agentpb.InspectExitStateResponse{
			Found: state != nil,
			State: exitStateToPB(state),
		}}

	case *agentpb.AgentRequest_Create:
		id, err := rt.Create(ctx, containerSpecFromPB(op.Create.GetSpec()))
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_Create{Create: &agentpb.CreateResponse{Id: id}}

	case *agentpb.AgentRequest_Start:
		if err := rt.Start(ctx, op.Start.GetId()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_Stop:
		timeout := time.Duration(op.Stop.GetTimeoutMs()) * time.Millisecond
		if op.Stop.GetTimeoutMs() < 0 {
			timeout = -1 // internal/docker.Runtime.Stop: negative means wait indefinitely
		}
		if err := rt.Stop(ctx, op.Stop.GetId(), timeout); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_Remove:
		if err := rt.Remove(ctx, op.Remove.GetId(), op.Remove.GetForce()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_UpdateResources:
		resources := resourcesFromPB(op.UpdateResources.GetResources())
		if resources == nil {
			resources = &docker.Resources{}
		}
		if err := rt.UpdateResources(ctx, op.UpdateResources.GetId(), *resources); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_ListImages:
		images, err := rt.ListImages(ctx, op.ListImages.GetRepo())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_ListImages{ListImages: &agentpb.ListImagesResponse{Images: imageInfosToPB(images)}}

	case *agentpb.AgentRequest_ListByPrefix:
		states, err := rt.ListByPrefix(ctx, op.ListByPrefix.GetPrefix())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_ListByPrefix{ListByPrefix: &agentpb.ListByPrefixResponse{Containers: containerStatesToPB(states)}}

	case *agentpb.AgentRequest_EnsureVolume:
		if err := rt.EnsureVolume(ctx, op.EnsureVolume.GetName()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_EnsureNetwork:
		id, err := rt.EnsureNetwork(ctx, op.EnsureNetwork.GetName())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_EnsureNetwork{EnsureNetwork: &agentpb.EnsureNetworkResponse{Id: id}}

	case *agentpb.AgentRequest_RemoveNetwork:
		if err := rt.RemoveNetwork(ctx, op.RemoveNetwork.GetName()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_NetworkConnect:
		if err := rt.NetworkConnect(ctx, op.NetworkConnect.GetNetwork(), op.NetworkConnect.GetContainerId()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_NetworkDisconnect:
		if err := rt.NetworkDisconnect(ctx, op.NetworkDisconnect.GetNetwork(), op.NetworkDisconnect.GetContainerId(), op.NetworkDisconnect.GetForce()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_RemoveVolume:
		remover, ok := rt.(docker.VolumeRemover)
		if !ok {
			resp.Error = ErrVolumeRemoveUnsupported.Error()
			return resp
		}
		if err := remover.RemoveVolume(ctx, op.RemoveVolume.GetName()); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}

	case *agentpb.AgentRequest_Logs:
		reader, ok := rt.(docker.LogReader)
		if !ok {
			resp.Error = ErrLogsUnsupported.Error()
			return resp
		}
		lines, err := readLogBatch(ctx, reader, op.Logs.GetContainerId(), op.Logs.GetSinceUnixNano())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_Logs{Logs: &agentpb.LogsResponse{Lines: lines}}

	case *agentpb.AgentRequest_Stats:
		inspector, ok := rt.(docker.StatsInspector)
		if !ok {
			resp.Error = ErrStatsUnsupported.Error()
			return resp
		}
		stats, err := inspector.Stats(ctx, op.Stats.GetContainerId())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_Stats{Stats: &agentpb.StatsResponse{Stats: containerStatsToPB(stats)}}

	case *agentpb.AgentRequest_ListNetworksByPrefix:
		networks, err := rt.ListNetworksByPrefix(ctx, op.ListNetworksByPrefix.GetPrefix())
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.Result = &agentpb.AgentResponse_ListNetworksByPrefix{ListNetworksByPrefix: &agentpb.ListNetworksByPrefixResponse{
			Networks: networkInfosToPB(networks),
		}}

	case *agentpb.AgentRequest_WatchEvents:
		watchID := op.WatchEvents.GetWatchId()
		go relayEvents(ctx, rt, watchID, emitEvent)
		resp.Result = emptyResult()

	case *agentpb.AgentRequest_Exec:
		// Exec is the one op Execute does not own: it spans many frames
		// in both directions, so ExecRelay below holds its per-session
		// state and serveSession routes it there directly.
		resp.Error = "agent: exec requests are dispatched through ExecRelay, not Execute"

	case *agentpb.AgentRequest_Build:
		// Same as exec: a build spans many frames in both directions, so
		// BuildRelay holds its per-session state and serveSession routes
		// it there directly.
		resp.Error = "agent: build requests are dispatched through BuildRelay, not Execute"

	case *agentpb.AgentRequest_ApplyMesh:
		// Mesh ops are dispatched in serveSession, since they need the node ID and MeshApplier.
		resp.Error = "agent: apply-mesh requests are dispatched directly by serveSession, not Execute"

	case *agentpb.AgentRequest_RotateMeshKey:
		resp.Error = "agent: rotate-mesh-key requests are dispatched directly by serveSession, not Execute"

	default:
		resp.Error = fmt.Sprintf("agent: unknown request op %T", op)
	}

	return resp
}

// ErrExitStateUnsupported means the runtime cannot inspect exit state; callers wait out the full readiness budget.
var ErrExitStateUnsupported = errors.New("agent: this node's container runtime cannot inspect container exit state")

// ErrImageLoadUnsupported is returned when the node runtime cannot load an image archive.
var ErrImageLoadUnsupported = errors.New("agent: this node's container runtime cannot load an image archive")

// ErrStatsUnsupported is what a Stats request gets when this node's
// runtime cannot report container resource usage.
var ErrStatsUnsupported = errors.New("agent: this node's container runtime cannot report container stats")

func emptyResult() *agentpb.AgentResponse_Empty {
	return &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}
}

// Events are pushed up to the control plane as they happen, per ADR 003, rather than polled.
func relayEvents(ctx context.Context, rt docker.Runtime, watchID string, emitEvent func(*agentpb.ProxiedEvent)) {
	events, errs := rt.Events(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			emitEvent(&agentpb.ProxiedEvent{
				WatchId:       watchID,
				Action:        string(ev.Action),
				ContainerName: ev.ContainerName,
				Time:          timestamppb.New(ev.Time),
			})
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			// A stream error has no pending request to attach to, so the watch simply ends.
			return
		}
	}
}
