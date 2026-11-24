package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Join token lifetime settings, in minutes. The constants are only the seed defaults.
const (
	KeyNodesJoinTokenTTLMinutes     = "nodes.join_token_ttl_minutes"
	KeyNodesJoinTokenLongTTLMinutes = "nodes.join_token_long_ttl_minutes"

	minJoinTTLMinutes = 1
	maxJoinTTLMinutes = 1440
)

func joinTTLMinutesCheck(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < minJoinTTLMinutes || n > maxJoinTTLMinutes {
		return fmt.Errorf("must be a whole number of minutes from %d to %d", minJoinTTLMinutes, maxJoinTTLMinutes)
	}
	return nil
}

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyNodesJoinTokenTTLMinutes, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: strconv.Itoa(int(nodeJoinTokenTTL / time.Minute)), Env: "APP_NODE_JOIN_TOKEN_TTL_MINUTES", Apply: runtimesettings.ApplyLive,
		CheckValue:  joinTTLMinutesCheck,
		Title:       "Join token lifetime",
		Description: "Minutes a node join or re-enrollment token stays redeemable, from 1 to 1440. Applies to tokens minted after the change.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyNodesJoinTokenLongTTLMinutes, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: strconv.Itoa(int(nodeProvisionJoinTokenTTL / time.Minute)), Env: "APP_NODE_JOIN_TOKEN_LONG_TTL_MINUTES", Apply: runtimesettings.ApplyLive,
		CheckValue:  joinTTLMinutesCheck,
		Title:       "Provisioned node join token lifetime",
		Description: "Minutes the join token minted for a cloud provision stays redeemable, from 1 to 1440. Keep it longer than the provision timeout. Applies to tokens minted after the change.",
	})
}

// joinTokenTTLFor reads the live lifetime; long is the cloud provision token.
func (rt *Router) joinTokenTTLFor(ctx context.Context, long bool) time.Duration {
	if rt.runtimeSettings == nil {
		if long {
			return nodeProvisionJoinTokenTTL
		}
		return rt.effectiveJoinTokenTTL()
	}
	key, def := KeyNodesJoinTokenTTLMinutes, nodeJoinTokenTTL
	if long {
		key, def = KeyNodesJoinTokenLongTTLMinutes, nodeProvisionJoinTokenTTL
	}
	d := time.Duration(rt.runtimeSettings.Int(ctx, key)) * time.Minute
	if d < time.Duration(minJoinTTLMinutes)*time.Minute || d > time.Duration(maxJoinTTLMinutes)*time.Minute {
		return def
	}
	// The older startup option still wins while the setting sits at its default.
	if !long && d == nodeJoinTokenTTL {
		return rt.effectiveJoinTokenTTL()
	}
	return d
}
