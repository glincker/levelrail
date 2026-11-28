package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/janitor"
	"github.com/glincker/levelrail/internal/nodecapacity"
	"github.com/glincker/levelrail/internal/store"
)

// CapNodeCleanup means the agent frees disk on its own Docker when asked. An older
// agent never receives the request and the control plane reports cleanup as unsupported.
const CapNodeCleanup = "node_cleanup"

// ErrCleanupUnsupported is returned when the node runtime cannot prune.
var ErrCleanupUnsupported = errors.New("agent: this node's container runtime cannot run a capacity cleanup")

// agentJanitorStore gives the shared engine the two calls the safe stages make, with no persistence on the node.
// The control plane records the run.
type agentJanitorStore struct{ janitor.Store }

func (agentJanitorStore) GetJanitorPolicy(context.Context) (string, bool, error) {
	return "", false, nil
}

func (agentJanitorStore) InsertJanitorRun(context.Context, store.JanitorRun, int) error { return nil }

func executeCleanup(ctx context.Context, rt docker.Runtime, req *agentpb.CleanupRequest) (*agentpb.CleanupResponse, error) {
	dk, ok := rt.(janitor.Docker)
	if !ok {
		return nil, ErrCleanupUnsupported
	}
	keep := req.GetKeepContainers()
	eng := janitor.New(janitor.Deps{
		Store:  agentJanitorStore{},
		Docker: dk,
		DiskFree: func(context.Context) (int64, int64, error) {
			return 0, 0, errors.New("not measured on the node")
		},
		DesiredContainers: func(context.Context) ([]string, error) { return keep, nil },
		Now:               time.Now,
	})
	if req.GetDryRun() {
		p, err := nodecapacity.BuildSafePreview(ctx, eng, dk)
		if err != nil {
			return nil, err
		}
		return &agentpb.CleanupResponse{Classes: classesToPB(p.Classes), TotalBytes: p.TotalBytes}, nil
	}
	res, err := nodecapacity.Execute(ctx, eng, janitor.TriggerNodeGuardrail)
	if err != nil {
		return nil, err
	}
	return &agentpb.CleanupResponse{Classes: classesToPB(res.Classes), TotalBytes: res.TotalBytes, FreedBytes: res.FreedBytes, Error: res.Error}, nil
}

func classesToPB(in []nodecapacity.Class) []*agentpb.CleanupClass {
	out := make([]*agentpb.CleanupClass, 0, len(in))
	for _, c := range in {
		pc := &agentpb.CleanupClass{Name: c.Name, Deletable: c.Deletable, Bytes: c.Bytes}
		for _, it := range c.Items {
			pc.Items = append(pc.Items, &agentpb.CleanupItem{Name: it.Name, Detail: it.Detail, SizeBytes: it.SizeBytes, Action: it.Action})
		}
		out = append(out, pc)
	}
	return out
}

func classesFromPB(in []*agentpb.CleanupClass) []nodecapacity.Class {
	out := make([]nodecapacity.Class, 0, len(in))
	for _, c := range in {
		nc := nodecapacity.Class{Name: c.GetName(), Deletable: c.GetDeletable(), Bytes: c.GetBytes(), Items: []janitor.Item{}}
		for _, it := range c.GetItems() {
			nc.Items = append(nc.Items, janitor.Item{Name: it.GetName(), Detail: it.GetDetail(), SizeBytes: it.GetSizeBytes(), Action: it.GetAction()})
		}
		out = append(out, nc)
	}
	return out
}

// NodeCleanup previews or runs the safe capacity cleanup on this node's own Docker.
// keep names desired containers the node must not remove. Nothing is sent to an agent without the capability.
func (t *GRPCTransport) NodeCleanup(ctx context.Context, dryRun bool, keep []string) (nodecapacity.Result, error) {
	if !t.caps.has(CapNodeCleanup) {
		return nodecapacity.Result{}, ErrAgentLacksCapability
	}
	resp, err := t.destructive(ctx, "cleanup", func(tok uint64, tg string) *agentpb.AgentRequest {
		return &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Cleanup{
			Cleanup: &agentpb.CleanupRequest{DryRun: dryRun, KeepContainers: keep, FenceToken: tok, FenceTarget: tg},
		}}
	})
	if err != nil {
		return nodecapacity.Result{}, fmt.Errorf("agent: node cleanup: %w", err)
	}
	r := resp.GetCleanup()
	if r == nil {
		return nodecapacity.Result{}, errors.New("agent: node cleanup: empty response")
	}
	return nodecapacity.Result{
		Preview:    nodecapacity.Preview{Classes: classesFromPB(r.GetClasses()), TotalBytes: r.GetTotalBytes()},
		FreedBytes: r.GetFreedBytes(), Error: r.GetError(),
	}, nil
}

// CanCleanup reports whether this node's agent advertises cleanup.
func (t *GRPCTransport) CanCleanup() bool { return t.caps.has(CapNodeCleanup) }
