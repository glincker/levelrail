package runtimesettings

// Agent offline buffer and self-update health limits.
const (
	KeyAgentBufferMaxItems     = "nodes.agent_buffer_max_items"
	KeyAgentBufferMaxBytes     = "nodes.agent_buffer_max_bytes"
	KeyAgentUpdateHealthFailed = "nodes.agent_update_health_failures"
	KeyNodeDrainStallSeconds   = "nodes.drain_stall_seconds"
	agentBufferItemsCeiling    = 100000
	agentBufferBytesCeiling    = 256 << 20
	agentUpdateFailuresCeiling = 20
)

func init() {
	Register(Definition{
		Key: KeyAgentBufferMaxItems, Category: CategoryObservability, Kind: KindInt, Default: "2000",
		Env: "APP_NODE_AGENT_BUFFER_MAX_ITEMS", Apply: ApplyLive, CheckValue: intRangeCheck(0, agentBufferItemsCeiling),
		Title:       "Agent offline buffer samples",
		Description: "Most metric and event samples an agent keeps while the control plane is unreachable, replayed oldest first on reconnect. 0 turns buffering off.",
	})
	Register(Definition{
		Key: KeyAgentBufferMaxBytes, Category: CategoryObservability, Kind: KindInt, Default: "4194304",
		Env: "APP_NODE_AGENT_BUFFER_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(0, agentBufferBytesCeiling),
		Title:       "Agent offline buffer bytes",
		Description: "Most bytes an agent holds in its offline buffer. The oldest samples are dropped first. 0 turns buffering off.",
	})
	Register(Definition{
		Key: KeyAgentUpdateHealthFailed, Category: CategoryObservability, Kind: KindInt, Default: "3",
		Env: "APP_NODE_AGENT_UPDATE_HEALTH_FAILURES", Apply: ApplyLive, CheckValue: intRangeCheck(1, agentUpdateFailuresCeiling),
		Title:       "Agent update rollback after failures",
		Description: "Consecutive failed health checks after an agent update before the update check tells the agent to return to its last good version, from 1 to 20.",
	})
	Register(Definition{
		Key: KeyNodeDrainStallSeconds, Category: CategoryObservability, Kind: KindInt, Default: "600",
		Env: "APP_NODE_DRAIN_STALL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(30, 86400),
		Title:       "Node drain stall seconds",
		Description: "Seconds a running node drain may go without progress before it is shown as stalled, from 30 to 86400.",
	})
}
