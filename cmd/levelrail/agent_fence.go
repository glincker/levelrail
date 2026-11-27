package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// agentFenceOption connects the node transports to the placement tokens and the live agent.fence_required setting.
func agentFenceOption(db *store.DB, logger *slog.Logger) agent.Option {
	settings := runtimesettings.New(db, nil)
	return agent.WithFenceHooks(agent.FenceHooks{
		Required: func(ctx context.Context) bool { return settings.Bool(ctx, runtimesettings.KeyAgentFenceRequired) },
		Current:  db.ServiceFenceToken,
		Refused: func(ctx context.Context, nodeID, target string, sent uint64, op, detail string) {
			logger.Warn("stale_fence_refused", slog.String("node_id", nodeID), slog.String("target", target),
				slog.Uint64("sent_token", sent), slog.String("op", op))
			ev := store.NodeSecurityEvent{
				NodeID: nodeID, Kind: store.SecurityKindStaleAction, Outcome: store.SecurityOutcomeRejected,
				Reason: "stale_fence_refused: " + op + " on " + target + ": " + detail,
			}
			if err := db.AddNodeSecurityEvent(ctx, ev, store.SecurityEventPolicy{}, time.Now()); err != nil {
				logger.Warn("stale_fence_refused: event not stored", slog.String("error", err.Error()))
			}
		},
		Rejected: func(ctx context.Context, nodeID, target string, sent, current uint64, op string) {
			logger.Warn("stale_action_rejected", slog.String("node_id", nodeID), slog.String("target", target),
				slog.Uint64("sent_token", sent), slog.Uint64("current_token", current), slog.String("op", op))
			ev := store.NodeSecurityEvent{
				NodeID: nodeID, Kind: store.SecurityKindStaleAction, Outcome: store.SecurityOutcomeRejected,
				Reason: "stale_action_rejected: " + op + " on " + target,
			}
			if err := db.AddNodeSecurityEvent(ctx, ev, store.SecurityEventPolicy{}, time.Now()); err != nil {
				logger.Warn("stale_action_rejected: event not stored", slog.String("error", err.Error()))
			}
		},
	})
}
