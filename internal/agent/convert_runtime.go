package agent

import (
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

func runtimeToPB(s docker.ContainerSpec, pb *agentpb.ContainerSpec) *agentpb.ContainerSpec {
	pb.Tmpfs, pb.Init, pb.Sysctls, pb.CapDrop, pb.PidsLimit = s.Tmpfs, s.Init, s.Sysctls, s.CapDrop, s.PidsLimit
	if s.StopTimeoutSeconds != nil {
		v := int32(min(*s.StopTimeoutSeconds, spec.MaxStopTimeoutSeconds)) //nolint:gosec // bounded by spec validation
		pb.StopTimeoutSeconds = &v
	}
	for _, u := range s.Ulimits {
		pb.Ulimits = append(pb.Ulimits, &agentpb.Ulimit{Name: u.Name, Soft: u.Soft, Hard: u.Hard})
	}
	return pb
}

func runtimeFromPB(pb *agentpb.ContainerSpec, s docker.ContainerSpec) docker.ContainerSpec {
	s.Tmpfs, s.Init, s.Sysctls, s.CapDrop, s.PidsLimit = pb.Tmpfs, pb.Init, pb.Sysctls, pb.CapDrop, pb.PidsLimit
	if pb.StopTimeoutSeconds != nil {
		v := int(*pb.StopTimeoutSeconds)
		s.StopTimeoutSeconds = &v
	}
	for _, u := range pb.Ulimits {
		s.Ulimits = append(s.Ulimits, docker.Ulimit{Name: u.GetName(), Soft: u.GetSoft(), Hard: u.GetHard()})
	}
	return s
}
