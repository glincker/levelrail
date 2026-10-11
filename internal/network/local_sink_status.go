package network

import "context"

// Status reports the local device's live state, for node-pair diagnostics.
func (s *LocalSink) Status(ctx context.Context) (Status, error) {
	return s.mesh.Status(ctx)
}
