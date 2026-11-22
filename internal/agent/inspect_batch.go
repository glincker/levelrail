package agent

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/docker"
)

var _ docker.BatchInspector = (*GRPCTransport)(nil)

// InspectManyByName answers many lookups with one list round trip, so a pass
// over a remote node costs one agent call instead of one per app. It uses the
// existing list op, so no agent upgrade is needed.
func (t *GRPCTransport) InspectManyByName(ctx context.Context, names []string) (map[string]*docker.ContainerState, error) {
	all, err := t.ListByPrefix(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]*docker.ContainerState, len(names))
	for _, n := range names {
		out[n] = nil
	}
	for i := range all {
		if _, asked := out[all[i].Name]; asked && out[all[i].Name] == nil {
			cs := all[i]
			out[cs.Name] = &cs
		}
	}
	return out, nil
}
