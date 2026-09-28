package gpu

import (
	"context"
	"log/slog"
	"time"
)

// Sink stores a node's GPU snapshot.
type Sink interface {
	SetNodeGPU(ctx context.Context, nodeID string, info Info) error
}

// RunCollector detects this host's GPUs immediately and then every
// interval, writing each snapshot for nodeID to sink until ctx ends.
func RunCollector(ctx context.Context, sink Sink, r Runner, rl RuntimeLister, nodeID string, interval time.Duration, logger *slog.Logger) {
	collect := func() {
		if err := sink.SetNodeGPU(ctx, nodeID, Detect(ctx, r, rl)); err != nil && ctx.Err() == nil {
			logger.Warn("gpu: store local node snapshot failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		}
	}
	collect()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}
