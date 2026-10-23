package build

import (
	"fmt"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/opencontainers/go-digest"
)

// CgroupProvisioner creates a resource-capped cgroup for one build. parent is
// the value BuildKit takes as its cgroup parent; cleanup removes the cgroup.
type CgroupProvisioner interface {
	Provision(id string, memoryMB, cpuMillis int) (parent string, cleanup func(), err error)
}

// withCgroupParent sets the cgroup parent on every exec op. An op's digest
// covers its content, so each rewritten op gets a new digest and the inputs
// and metadata of the ops after it follow.
func withCgroupParent(def *llb.Definition, parent string) (*llb.Definition, error) {
	remap := make(map[digest.Digest]digest.Digest, len(def.Def))
	out := &llb.Definition{
		Def:         make([][]byte, 0, len(def.Def)),
		Metadata:    make(map[digest.Digest]llb.OpMetadata, len(def.Metadata)),
		Constraints: def.Constraints,
	}
	for _, raw := range def.Def {
		var op pb.Op
		if err := op.UnmarshalVT(raw); err != nil {
			return nil, fmt.Errorf("decode op: %w", err)
		}
		for _, in := range op.Inputs {
			if nd, ok := remap[digest.Digest(in.Digest)]; ok {
				in.Digest = string(nd)
			}
		}
		if ex := op.GetExec(); ex != nil && ex.Meta != nil {
			ex.Meta.CgroupParent = parent
		}
		enc, err := op.MarshalVT()
		if err != nil {
			return nil, fmt.Errorf("encode op: %w", err)
		}
		remap[digest.FromBytes(raw)] = digest.FromBytes(enc)
		out.Def = append(out.Def, enc)
	}
	for d, m := range def.Metadata {
		if nd, ok := remap[d]; ok {
			d = nd
		}
		out.Metadata[d] = m
	}
	if def.Source != nil {
		src := &pb.Source{Infos: def.Source.Infos, Locations: make(map[string]*pb.Locations, len(def.Source.Locations))}
		for d, l := range def.Source.Locations {
			if nd, ok := remap[digest.Digest(d)]; ok {
				d = string(nd)
			}
			src.Locations[d] = l
		}
		out.Source = src
	}
	return out, nil
}
