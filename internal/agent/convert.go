package agent

// This file: pure conversions between internal/docker's Go types and
// internal/agent/agentpb's generated proto types. Field-for-field
// mirrors, deliberately mechanical: proto.go's own header comment
// already explains why the message shapes exist, this file is just the
// translation layer both the control-plane dispatcher (grpc_transport.go)
// and the agent-side executor (execute.go) share, so the mapping is
// defined exactly once.

import (
	"math"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

func portBindingToPB(p docker.PortBinding) *agentpb.PortBinding {
	return &agentpb.PortBinding{
		ContainerPort: int32(p.ContainerPort), //nolint:gosec // port numbers fit int32 by construction (0-65535)
		HostPort:      int32(p.HostPort),      //nolint:gosec // same
		Protocol:      p.Protocol,
	}
}

func portBindingFromPB(p *agentpb.PortBinding) docker.PortBinding {
	if p == nil {
		return docker.PortBinding{}
	}
	return docker.PortBinding{
		ContainerPort: int(p.ContainerPort),
		HostPort:      int(p.HostPort),
		Protocol:      p.Protocol,
	}
}

func portBindingsToPB(ps []docker.PortBinding) []*agentpb.PortBinding {
	if ps == nil {
		return nil
	}
	out := make([]*agentpb.PortBinding, len(ps))
	for i, p := range ps {
		out[i] = portBindingToPB(p)
	}
	return out
}

func portBindingsFromPB(ps []*agentpb.PortBinding) []docker.PortBinding {
	if ps == nil {
		return nil
	}
	out := make([]docker.PortBinding, len(ps))
	for i, p := range ps {
		out[i] = portBindingFromPB(p)
	}
	return out
}

func resourcesToPB(r *docker.Resources) *agentpb.Resources {
	if r == nil {
		return nil
	}
	return &agentpb.Resources{
		MemoryBytes:     r.MemoryBytes,
		NanoCpus:        r.NanoCPUs,
		SwapMemoryBytes: r.SwapMemoryBytes,
		CpusetCpus:      r.CPUSetCPUs,
	}
}

func resourcesFromPB(r *agentpb.Resources) *docker.Resources {
	if r == nil {
		return nil
	}
	return &docker.Resources{
		MemoryBytes:     r.MemoryBytes,
		NanoCPUs:        r.NanoCpus,
		SwapMemoryBytes: r.SwapMemoryBytes,
		CPUSetCPUs:      r.CpusetCpus,
	}
}

func volumesToPB(vs []docker.VolumeMount) []*agentpb.VolumeMount {
	if vs == nil {
		return nil
	}
	out := make([]*agentpb.VolumeMount, len(vs))
	for i, v := range vs {
		out[i] = &agentpb.VolumeMount{Name: v.Name, ContainerPath: v.ContainerPath}
	}
	return out
}

func volumesFromPB(vs []*agentpb.VolumeMount) []docker.VolumeMount {
	if vs == nil {
		return nil
	}
	out := make([]docker.VolumeMount, len(vs))
	for i, v := range vs {
		out[i] = docker.VolumeMount{Name: v.GetName(), ContainerPath: v.GetContainerPath()}
	}
	return out
}

func containerSpecToPB(s docker.ContainerSpec) *agentpb.ContainerSpec {
	return &agentpb.ContainerSpec{
		Name:      s.Name,
		Image:     s.Image,
		Ports:     portBindingsToPB(s.Ports),
		Env:       s.Env,
		Resources: resourcesToPB(s.Resources),
		Volumes:   volumesToPB(s.Volumes),
		Dns:       s.DNS,
	}
}

func containerSpecFromPB(s *agentpb.ContainerSpec) docker.ContainerSpec {
	if s == nil {
		return docker.ContainerSpec{}
	}
	return docker.ContainerSpec{
		Name:      s.Name,
		Image:     s.Image,
		Ports:     portBindingsFromPB(s.Ports),
		Env:       s.Env,
		Resources: resourcesFromPB(s.Resources),
		Volumes:   volumesFromPB(s.Volumes),
		DNS:       s.Dns,
	}
}

func containerStateToPB(s *docker.ContainerState) *agentpb.ContainerState {
	if s == nil {
		return nil
	}
	return &agentpb.ContainerState{
		Id:      s.ID,
		Name:    s.Name,
		Image:   s.Image,
		Running: s.Running,
		Ports:   portBindingsToPB(s.Ports),
	}
}

func containerStateFromPB(s *agentpb.ContainerState) *docker.ContainerState {
	if s == nil {
		return nil
	}
	return &docker.ContainerState{
		ID:      s.Id,
		Name:    s.Name,
		Image:   s.Image,
		Running: s.Running,
		Ports:   portBindingsFromPB(s.Ports),
	}
}

func containerStatesFromPB(ss []*agentpb.ContainerState) []docker.ContainerState {
	if ss == nil {
		return nil
	}
	out := make([]docker.ContainerState, len(ss))
	for i, s := range ss {
		out[i] = *containerStateFromPB(s)
	}
	return out
}

func containerStatesToPB(ss []docker.ContainerState) []*agentpb.ContainerState {
	if ss == nil {
		return nil
	}
	out := make([]*agentpb.ContainerState, len(ss))
	for i := range ss {
		out[i] = containerStateToPB(&ss[i])
	}
	return out
}

func imageInfoToPB(i docker.ImageInfo) *agentpb.ImageInfo {
	return &agentpb.ImageInfo{Tag: i.Tag, CreatedAt: timestamppb.New(i.CreatedAt)}
}

func imageInfoFromPB(i *agentpb.ImageInfo) docker.ImageInfo {
	if i == nil {
		return docker.ImageInfo{}
	}
	return docker.ImageInfo{Tag: i.Tag, CreatedAt: timestampFromPB(i.CreatedAt)}
}

func imageInfosToPB(is []docker.ImageInfo) []*agentpb.ImageInfo {
	if is == nil {
		return nil
	}
	out := make([]*agentpb.ImageInfo, len(is))
	for i, info := range is {
		out[i] = imageInfoToPB(info)
	}
	return out
}

func imageInfosFromPB(is []*agentpb.ImageInfo) []docker.ImageInfo {
	if is == nil {
		return nil
	}
	out := make([]docker.ImageInfo, len(is))
	for i, info := range is {
		out[i] = imageInfoFromPB(info)
	}
	return out
}

func networkInfosToPB(ns []docker.NetworkInfo) []*agentpb.NetworkInfo {
	if ns == nil {
		return nil
	}
	out := make([]*agentpb.NetworkInfo, len(ns))
	for i, n := range ns {
		out[i] = &agentpb.NetworkInfo{Id: n.ID, Name: n.Name}
	}
	return out
}

func networkInfosFromPB(ns []*agentpb.NetworkInfo) []docker.NetworkInfo {
	if ns == nil {
		return nil
	}
	out := make([]docker.NetworkInfo, len(ns))
	for i, n := range ns {
		out[i] = docker.NetworkInfo{ID: n.GetId(), Name: n.GetName()}
	}
	return out
}

func timestampFromPB(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func ttySizeFromPB(s *agentpb.ExecTTYSize) docker.TTYSize {
	return docker.TTYSize{Rows: clampTTYDimension(s.GetRows()), Cols: clampTTYDimension(s.GetCols())}
}

func ttySizeToPB(s docker.TTYSize) *agentpb.ExecTTYSize {
	return &agentpb.ExecTTYSize{Rows: uint32(s.Rows), Cols: uint32(s.Cols)}
}

// clampTTYDimension narrows the wire's uint32 to the uint16 a terminal
// dimension actually is, since no PTY has 65536 rows and a peer sending
// one must not wrap around to a small number.
func clampTTYDimension(v uint32) uint16 {
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(v)
}
