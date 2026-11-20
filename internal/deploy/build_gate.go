package deploy

import "context"

// BuildGate vets an image a remote builder produced before it becomes desired state.
type BuildGate interface {
	CheckBuild(ctx context.Context, app, attemptID, recordID, digest, tier, nodeID string) error
}

// WithBuildGate refuses images from builders the environment disallows or whose signed record fails.
func WithBuildGate(g BuildGate) Option {
	return func(p *Pipeline) { p.buildGate = g }
}
