package agent

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// RunToCompletion implements docker.RunToCompletionRuntime by forwarding to
// the wrapped runtime, which the embedded docker.Runtime would hide.
func (l Local) RunToCompletion(ctx context.Context, spec docker.ContainerSpec, maxOutput int) (docker.RunOutcome, error) {
	return docker.RunToCompletionVia(ctx, l.Runtime, spec, maxOutput)
}
