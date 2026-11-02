package runtimesettings

import "time"

// KeyComposeDependsOnTimeout bounds how long a compose service waits on a depends_on dependency.
const KeyComposeDependsOnTimeout = "compose.depends_on_timeout"

const (
	minDependsOnTimeout = 5 * time.Second
	maxDependsOnTimeout = time.Hour
)

func init() {
	Register(Definition{
		Key: KeyComposeDependsOnTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "120s",
		Env: "APP_COMPOSE_DEPENDS_ON_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(minDependsOnTimeout, maxDependsOnTimeout),
		Title:       "Compose dependency wait",
		Description: "How long a compose service waits for a depends_on dependency to start, pass its health check or exit 0 before the deploy fails naming that dependency, from 5s to 1h; a change applies on the next reconcile pass.",
	})
}
