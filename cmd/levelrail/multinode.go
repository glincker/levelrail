package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/nodemove"
	"github.com/GLINCKER/levelrail/internal/store"
)

// multiNodeOption enables persisted node moves and drains, tuned by
// APP_NODE_MOVE_* and APP_MESH_DB_PORT_* (see docs/multi-node.md).
func multiNodeOption(db *store.DB) api.Option {
	lo, hi := meshpath.DatabasePortRange(os.LookupEnv)
	return api.WithMultiNode(db, nodemove.ConfigFromEnv(os.LookupEnv), lo, hi)
}

// nodeJoinTokenTTL reads APP_NODE_JOIN_TOKEN_TTL; 0 keeps the default.
func nodeJoinTokenTTL(logger *slog.Logger) time.Duration {
	v, ok := os.LookupEnv("APP_NODE_JOIN_TOKEN_TTL")
	if !ok || v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		logger.Warn("ignoring invalid APP_NODE_JOIN_TOKEN_TTL", slog.String("value", v))
		return 0
	}
	return d
}
