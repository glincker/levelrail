package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/docker"
)

// CapVolumeList means the agent lists its own named volumes when asked. An older
// agent never receives the request, so ownership checks on its node fail closed.
const CapVolumeList = "volume_list"

// ErrVolumeListUnsupported is returned when the node runtime cannot list volumes.
var ErrVolumeListUnsupported = errors.New("agent: this node's container runtime cannot list volumes")

type namedVolumeLister interface {
	ListNamedVolumes(ctx context.Context) ([]docker.NamedVolume, error)
}

func executeListVolumes(ctx context.Context, rt docker.Runtime) (*agentpb.ListVolumesResponse, error) {
	l, ok := rt.(namedVolumeLister)
	if !ok {
		return nil, ErrVolumeListUnsupported
	}
	vols, err := l.ListNamedVolumes(ctx)
	if err != nil {
		return nil, err
	}
	out := &agentpb.ListVolumesResponse{Volumes: make([]*agentpb.VolumeEntry, 0, len(vols))}
	for _, v := range vols {
		out.Volumes = append(out.Volumes, &agentpb.VolumeEntry{Name: v.Name, Mounted: v.Mounted})
	}
	return out, nil
}

// ListNodeVolumes asks the node for its named volumes. The name differs from
// ListNamedVolumes on purpose: reconcilers treat a runtime with that method as
// able to list on every pass, which an older agent cannot do.
func (t *GRPCTransport) ListNodeVolumes(ctx context.Context) ([]docker.NamedVolume, error) {
	if !t.caps.has(CapVolumeList) {
		return nil, ErrAgentLacksCapability
	}
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_ListVolumes{ListVolumes: &agentpb.ListVolumesRequest{}}})
	if err != nil {
		return nil, fmt.Errorf("agent: list volumes: %w", err)
	}
	r := resp.GetListVolumes()
	if r == nil {
		return nil, errors.New("agent: list volumes: empty response")
	}
	out := make([]docker.NamedVolume, 0, len(r.GetVolumes()))
	for _, v := range r.GetVolumes() {
		out = append(out, docker.NamedVolume{Name: v.GetName(), Mounted: v.GetMounted(), SizeBytes: -1})
	}
	return out, nil
}
