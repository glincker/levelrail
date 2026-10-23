package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// tickListing answers many prefix queries from one container listing, so a
// collector tick over N services costs one daemon call per node, not N.
type tickListing struct {
	rt     docker.Runtime
	all    []docker.ContainerState
	loaded bool
}

func (t *tickListing) withPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	if !t.loaded {
		all, err := t.rt.ListByPrefix(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("list containers: %w", err)
		}
		t.all, t.loaded = all, true
	}
	var out []docker.ContainerState
	for _, cs := range t.all {
		if strings.HasPrefix(cs.Name, prefix) {
			out = append(out, cs)
		}
	}
	return out, nil
}
