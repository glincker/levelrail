package main

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// advertiseHostFromStore prefers the address saved by the fix-it engine over APP_AGENT_ADVERTISE_HOST.
func advertiseHostFromStore(ctx context.Context, db *store.DB) string {
	if db != nil {
		if v, ok, err := db.GetFixitSetting(ctx, "agent_advertise_host"); err == nil && ok && v != "" {
			return v
		}
	}
	return agentAdvertiseHost()
}
