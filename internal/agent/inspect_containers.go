package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// CapContainerInspect means the agent answers InspectContainers; an older
// agent never receives the request and callers keep their unavailable markers.
const CapContainerInspect = "container_inspect"

// Agent-local bounds, read per request so an operator can lower them
// without trusting whatever limit the control plane asks for.
const (
	envInspectMaxContainers     = "APP_AGENT_INSPECT_MAX_CONTAINERS"
	envInspectMaxBytes          = "APP_AGENT_INSPECT_MAX_BYTES"
	defaultInspectMaxContainers = 50
	defaultInspectMaxBytes      = 256 << 10
	// ceilingInspectMaxBytes keeps one answer far below gRPC's 4 MiB receive limit.
	ceilingInspectMaxBytes = 1 << 20
)

// Per-container list caps the control plane applies to whatever an agent
// sends, so a compromised agent cannot make the dashboard render unbounded data.
const (
	maxDetailNetworks = 32
	maxDetailMounts   = 64
	maxDetailAliases  = 16
	maxDetailPorts    = 64
)

func agentEnvInt(name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && n > 0 {
		return n
	}
	return def
}

func executeInspectContainers(ctx context.Context, rt docker.Runtime, req *agentpb.InspectContainersRequest) (*agentpb.InspectContainersResponse, error) {
	inspector, ok := rt.(docker.ContainerDetailInspector)
	if !ok {
		return nil, docker.ErrDetailUnsupported
	}
	instance, prefix := strings.TrimSpace(req.GetInstanceId()), req.GetNamePrefix()
	if instance == "" || prefix == "" {
		return nil, docker.ErrDetailUnscoped
	}
	ids := map[string]bool{}
	for _, id := range req.GetContainerIds() {
		ids[id] = true
	}
	limit := agentEnvInt(envInspectMaxContainers, defaultInspectMaxContainers)
	if l := int(req.GetLimit()); l > 0 && l < limit {
		limit = l
	}
	details, err := inspector.InspectContainerDetails(ctx, docker.DetailQuery{
		NamePrefix: prefix, LabelKey: spec.InstanceLabelKey, LabelValue: instance, Limit: limit, IDs: req.GetContainerIds(),
	})
	if err != nil && len(details) == 0 {
		return nil, fmt.Errorf("agent: inspect containers: %w", err)
	}
	budget := min(agentEnvInt(envInspectMaxBytes, defaultInspectMaxBytes), ceilingInspectMaxBytes)
	out := &agentpb.InspectContainersResponse{Truncated: err != nil}
	size := 0
	for _, d := range details {
		// Recheck scope here: a runtime that ignored the filter must not leak.
		if d.InstanceID != instance || !strings.HasPrefix(d.Name, prefix) || (len(ids) > 0 && !ids[d.ID]) {
			continue
		}
		if len(out.Containers) >= limit {
			out.Truncated = true
			break
		}
		pb := containerDetailToPB(d)
		n := proto.Size(pb)
		if size+n > budget {
			out.Truncated = true
			break
		}
		size += n
		out.Containers = append(out.Containers, pb)
	}
	return out, nil
}

func clampInt32(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	}
	return int32(n) // #nosec G115 -- range checked above
}

func containerDetailToPB(d docker.ContainerDetail) *agentpb.ContainerDetail {
	pb := &agentpb.ContainerDetail{
		Id: d.ID, Image: d.Image, ImageId: d.ImageID, RepoDigests: d.RepoDigests, State: d.State,
		RestartCount: clampInt32(d.RestartCount), Ports: portBindingsToPB(d.Ports),
		NanoCpus: d.NanoCPUs, MemoryBytes: d.MemoryBytes,
	}
	if h := d.Health; h != nil {
		pb.Health = &agentpb.ContainerHealthDetail{Status: h.Status, FailingStreak: clampInt32(h.FailingStreak), LastOutput: h.LastOutput}
	}
	if !d.StartedAt.IsZero() {
		pb.StartedAt = timestamppb.New(d.StartedAt)
	}
	if !d.FinishedAt.IsZero() {
		pb.FinishedAt = timestamppb.New(d.FinishedAt)
	}
	for _, n := range d.Networks {
		pb.Networks = append(pb.Networks, &agentpb.ContainerNetworkDetail{Name: n.Name, Ip: n.IP, Aliases: n.Aliases})
	}
	for _, m := range d.Mounts {
		pb.Mounts = append(pb.Mounts, &agentpb.ContainerMountDetail{Type: m.Type, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}
	return pb
}

func firstN[T any](list []T, n int) []T {
	if len(list) > n {
		return list[:n]
	}
	return list
}

func containerDetailFromPB(pb *agentpb.ContainerDetail, instance string) docker.ContainerDetail {
	d := docker.ContainerDetail{
		ID: pb.GetId(), Image: pb.GetImage(), ImageID: pb.GetImageId(), RepoDigests: pb.GetRepoDigests(), State: pb.GetState(),
		RestartCount: int(pb.GetRestartCount()), Ports: portBindingsFromPB(firstN(pb.GetPorts(), maxDetailPorts)),
		NanoCPUs: pb.GetNanoCpus(), MemoryBytes: pb.GetMemoryBytes(), InstanceID: instance,
		Networks: []docker.ContainerNetworkDetail{}, Mounts: []docker.ContainerMountDetail{},
	}
	if h := pb.GetHealth(); h != nil {
		d.Health = &docker.ContainerHealthDetail{Status: h.GetStatus(), FailingStreak: int(h.GetFailingStreak()), LastOutput: docker.CutUTF8(h.GetLastOutput(), docker.HealthOutputMaxBytes)}
	}
	if pb.GetStartedAt() != nil {
		d.StartedAt = pb.GetStartedAt().AsTime()
	}
	if pb.GetFinishedAt() != nil {
		d.FinishedAt = pb.GetFinishedAt().AsTime()
	}
	for _, n := range firstN(pb.GetNetworks(), maxDetailNetworks) {
		d.Networks = append(d.Networks, docker.ContainerNetworkDetail{Name: n.GetName(), IP: n.GetIp(), Aliases: firstN(n.GetAliases(), maxDetailAliases)})
	}
	for _, m := range firstN(pb.GetMounts(), maxDetailMounts) {
		d.Mounts = append(d.Mounts, docker.ContainerMountDetail{Type: m.GetType(), Source: m.GetSource(), Target: m.GetTarget(), ReadOnly: m.GetReadOnly()})
	}
	return d
}

// InspectContainerDetails implements docker.ContainerDetailInspector over
// the wire. An agent without the capability is never sent the request.
func (t *GRPCTransport) InspectContainerDetails(ctx context.Context, q docker.DetailQuery) ([]docker.ContainerDetail, error) {
	if !t.caps.has(CapContainerInspect) {
		return nil, docker.ErrDetailUnsupported
	}
	if q.NamePrefix == "" || q.LabelValue == "" {
		return nil, docker.ErrDetailUnscoped
	}
	limit := uint32(0)
	if q.Limit > 0 {
		limit = uint32(clampInt32(q.Limit)) // #nosec G115 -- clamped to a positive int32 first
	}
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_InspectContainers{
		InspectContainers: &agentpb.InspectContainersRequest{NamePrefix: q.NamePrefix, InstanceId: q.LabelValue, Limit: limit, ContainerIds: q.IDs},
	}})
	if err != nil {
		return nil, fmt.Errorf("agent: inspect containers: %w", err)
	}
	r := resp.GetInspectContainers()
	if r == nil {
		return nil, errors.New("agent: inspect containers: empty response")
	}
	out := make([]docker.ContainerDetail, 0, len(r.GetContainers()))
	for _, c := range r.GetContainers() {
		out = append(out, containerDetailFromPB(c, q.LabelValue))
	}
	if r.GetTruncated() {
		return out, ErrInspectTruncated
	}
	return out, nil
}

// ErrInspectTruncated comes with partial results when the agent's bounds cut the list.
var ErrInspectTruncated = errors.New("agent: inspect containers: result truncated by the agent's bounds")

var _ docker.ContainerDetailInspector = (*GRPCTransport)(nil)
