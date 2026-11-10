package runtimesettings

import "time"

// Mesh config push keys: how a node's join wakes the mesh and how long one push may take.
const (
	KeyMeshWakeDebounce    = "mesh.wake_debounce"
	KeyMeshApplyTimeout    = "mesh.apply_timeout"
	KeyMeshApplyRetries    = "mesh.apply_retries"
	KeyMeshApplyRetryDelay = "mesh.apply_retry_delay"
)

// Registered ranges; the clamps below apply them to env seeds too, which skip CheckValue.
const (
	meshWakeDebounceFloor = 100 * time.Millisecond
	meshWakeDebounceMax   = 30 * time.Second
	meshApplyTimeoutFloor = time.Second
	meshApplyTimeoutMax   = 5 * time.Minute
	meshRetryDelayFloor   = 50 * time.Millisecond
	meshRetryDelayMax     = 10 * time.Second
	meshApplyRetriesMax   = 5
)

// ClampMeshWakeDebounce bounds a mesh.wake_debounce value to its registered range.
func ClampMeshWakeDebounce(d time.Duration) time.Duration {
	return min(max(d, meshWakeDebounceFloor), meshWakeDebounceMax)
}

// ClampMeshApplyTimeout bounds a mesh.apply_timeout value to its registered range.
func ClampMeshApplyTimeout(d time.Duration) time.Duration {
	return min(max(d, meshApplyTimeoutFloor), meshApplyTimeoutMax)
}

// ClampMeshApplyRetryDelay bounds a mesh.apply_retry_delay value to its registered range.
func ClampMeshApplyRetryDelay(d time.Duration) time.Duration {
	return min(max(d, meshRetryDelayFloor), meshRetryDelayMax)
}

// ClampMeshApplyRetries bounds a mesh.apply_retries value to its registered range.
func ClampMeshApplyRetries(n int) int {
	return min(max(n, 0), meshApplyRetriesMax)
}

func init() {
	Register(Definition{
		Key: KeyMeshWakeDebounce, Category: CategoryNetworking, Kind: KindDuration, Default: "1s",
		Env: "APP_MESH_WAKE_DEBOUNCE", Apply: ApplyLive,
		CheckValue: durationRangeCheck(meshWakeDebounceFloor, meshWakeDebounceMax),
		Title:      "Mesh wake delay after a node joins or leaves",
		Description: "When a node connects, disconnects or reconnects from a new address, the control plane waits this long and then runs one reconcile pass, so the mesh updates in seconds instead of on the next resync. " +
			"Joins inside the window share one pass. The next join uses a changed value.",
	})
	Register(Definition{
		Key: KeyMeshApplyTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "10s",
		Env: "APP_MESH_APPLY_TIMEOUT", Apply: ApplyLive,
		CheckValue: durationRangeCheck(meshApplyTimeoutFloor, meshApplyTimeoutMax),
		Title:      "Mesh config push timeout",
		Description: "The longest one attempt to send a node its WireGuard mesh config may take, from 1s to 5m. An agent that does not answer in time is not retried: it is marked unreachable for this pass so the rest of the fleet still converges, and the next pass sends it the current config. " +
			"The next pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyMeshApplyRetries, Category: CategoryNetworking, Kind: KindInt, Default: "2",
		Env: "APP_MESH_APPLY_RETRIES", Apply: ApplyLive, CheckValue: intRangeCheck(0, meshApplyRetriesMax),
		Title:       "Mesh config push retries",
		Description: "How many more times, from 0 to 5, the control plane resends a node its mesh config when the agent session dropped mid push, using the node's new session. A timeout or a node that is not connected is not retried. The next pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyMeshApplyRetryDelay, Category: CategoryNetworking, Kind: KindDuration, Default: "500ms",
		Env: "APP_MESH_APPLY_RETRY_DELAY", Apply: ApplyLive,
		CheckValue: durationRangeCheck(meshRetryDelayFloor, meshRetryDelayMax),
		Title:      "Mesh config push retry delay",
		Description: "The wait before the first mesh config retry, from 50ms to 10s. Each further retry waits twice as long, up to the push timeout. " +
			"The next pass uses a changed value.",
	})
}
