package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// inspectBatchSize bounds the name filters in one list request.
const inspectBatchSize = 100

// BatchInspector is InspectByName for many names in one daemon call per batch.
type BatchInspector interface {
	InspectManyByName(ctx context.Context, names []string) (map[string]*ContainerState, error)
}

// InspectManyByName implements BatchInspector with InspectByName's exact
// semantics: anchored names, scoped to this instance, nil when absent.
func (c *Client) InspectManyByName(ctx context.Context, names []string) (map[string]*ContainerState, error) {
	out := make(map[string]*ContainerState, len(names))
	for _, n := range names {
		out[n] = nil
	}
	for start := 0; start < len(names); start += inspectBatchSize {
		end := min(start+inspectBatchSize, len(names))
		f := filters.NewArgs()
		for _, n := range names[start:end] {
			f.Add("name", "^/"+n+"$")
		}
		c.instanceLabelFilter(f)
		summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
		if err != nil {
			return nil, fmt.Errorf("docker: list %d containers by name: %w", end-start, err)
		}
		for _, s := range summaries {
			state := toContainerState(s)
			if _, asked := out[state.Name]; !asked || out[state.Name] != nil {
				continue
			}
			if !state.Running {
				state.ConfiguredPorts = c.configuredPorts(ctx, state.ID)
			}
			out[state.Name] = state
		}
	}
	return out, nil
}

// InspectSet is a set of container states read in one batch, answering
// InspectByName for the names it covers.
type InspectSet struct {
	states map[string]*ContainerState
}

// FetchInspectSet reads names through rt's BatchInspector. It returns nil when
// rt has none or the read fails, so callers fall back to per-name calls.
func FetchInspectSet(ctx context.Context, rt Runtime, names []string) *InspectSet {
	b, ok := rt.(BatchInspector)
	if !ok || len(names) == 0 {
		return nil
	}
	states, err := b.InspectManyByName(ctx, names)
	if err != nil {
		return nil
	}
	return &InspectSet{states: states}
}

// Lookup returns a private copy of name's state. covered is false for a name
// the batch never asked about; a covered name with a nil state does not exist.
func (s *InspectSet) Lookup(name string) (state *ContainerState, covered bool) {
	if s == nil {
		return nil, false
	}
	cs, covered := s.states[name]
	if !covered {
		return nil, false
	}
	if cs == nil {
		return nil, true
	}
	cp := *cs
	return &cp, true
}
