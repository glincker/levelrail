package agent

// This file: the agent-side executor. Pure with respect to networking
// (no gRPC, no streams): given one AgentRequest and a real
// docker.Runtime, produce the matching AgentResponse. What runs this
// against a real Session stream is a thin loop
// (cmd/levelrail-agent, TASKS.md 3.2's remaining wiring), kept
// separate specifically so this dispatch logic is testable with a
// hand-written fake docker.Runtime, no real network or real Docker
// daemon required.

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// Execute runs one AgentRequest against rt and returns the matching
// AgentResponse, always carrying req.RequestId. A docker.Runtime
// failure becomes AgentResponse.Error (a plain string over the wire,
// deliberately: this is a control-plane-to-agent RPC boundary, not a
// package boundary within one Go process, so wrapping with %w has
// nothing left to attach to on the receiving end; GRPCTransport
// reconstructs a plain error from this string instead), never a Go
// error return of Execute's own: this function's contract is "always
// produce exactly one response for exactly one request," success or
// failure alike, so the caller always has something to send back.
//
// ctx is the whole Session's own lifetime, not a fresh per-call one:
// WatchEvents in particular needs to keep running past this one call
// returning, for as long as the session itself stays open, and using
// the same ctx for every other operation keeps "when does this stop"
// answered identically everywhere in this function rather than
// inventing a second, shorter-lived context needing its own
// justification. emitEvent is only ever called from the WatchEvents
// path, unprompted, for as long as ctx stays alive.
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

	case *agentpb.AgentRequest_WatchEvents:
		watchID := op.WatchEvents.GetWatchId()
		go relayEvents(ctx, rt, watchID, emitEvent)
		resp.Result = emptyResult()

	default:
		resp.Error = fmt.Sprintf("agent: unknown request op %T", op)
	}

	return resp
}

func emptyResult() *agentpb.AgentResponse_Empty {
	return &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}
}

// relayEvents runs rt.Events(ctx) for the life of ctx, translating each
// docker.Event into a ProxiedEvent tagged with watchID and handing it to
// emitEvent. ADR 003's "the agent streams Docker events up to the
// control plane rather than the control plane polling down," made real.
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
			// A stream error here has no pending AgentRequest to
			// attach itself to: Execute already returned WatchEvents'
			// one acknowledgment before this goroutine started. A
			// dedicated error frame for this case is a real,
			// documented gap (TASKS.md 3.2's own write-up), not solved
			// in this pass; the watch simply ends, matching
			// docker.Runtime.Events' own "error channel receives at
			// most one error, then the event channel closes" contract.
			return
		}
	}
}
