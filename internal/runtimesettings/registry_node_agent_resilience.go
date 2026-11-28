package runtimesettings

import "github.com/glincker/levelrail/internal/updatewindow"

// Agent offline buffer and self-update health limits.
const (
	KeyAgentBufferMaxItems     = "nodes.agent_buffer_max_items"
	KeyAgentBufferMaxBytes     = "nodes.agent_buffer_max_bytes"
	KeyAgentUpdateHealthFailed = "nodes.agent_update_health_failures"
	KeyAgentAutoUpdate         = "nodes.agent_auto_update"
	KeyAgentUpdateWindow       = "nodes.agent_update_window"
	KeyAgentUpdateChannel      = "nodes.agent_update_channel"
	KeyAgentUpdateHeldNodes    = "nodes.agent_update_held_nodes"
	KeyAgentUpdateForceNodes   = "nodes.agent_update_force_nodes"
	KeyNodeDrainStallSeconds   = "nodes.drain_stall_seconds"
	KeyAgentFenceRequired      = "agent.fence_required"
	agentBufferItemsCeiling    = 100000
	agentBufferBytesCeiling    = 256 << 20
	agentUpdateFailuresCeiling = 20
)

func windowCheck(v string) error {
	_, err := updatewindow.Parse(v)
	return err
}

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
		Key: KeyAgentAutoUpdate, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_NODE_AGENT_AUTO_UPDATE", Apply: ApplyLive,
		Title:       "Agent automatic update",
		Description: "When on, agents that check for updates install the control plane version and restart themselves. Off by default: agents then only update when an operator runs levelrail-agent self-update --apply.",
	})
	Register(Definition{
		Key: KeyAgentUpdateWindow, Category: CategoryObservability, Kind: KindString, Default: "",
		Env: "APP_NODE_AGENT_UPDATE_WINDOW", Apply: ApplyLive, CheckValue: windowCheck,
		Title:       "Agent update window",
		Description: "Daily HH:MM-HH:MM range, in the control plane's local time, when automatic agent updates may install, for example 02:00-05:00. Empty means any time. An operator can still update anyway.",
	})
	Register(Definition{
		Key: KeyAgentUpdateChannel, Category: CategoryObservability, Kind: KindString, Default: "stable",
		Env: "APP_NODE_AGENT_UPDATE_CHANNEL", Apply: ApplyLive, AllowedStrings: []string{"stable", "hold-minor"},
		Title:       "Agent update channel",
		Description: "stable lets agents follow the control plane version automatically. hold-minor only installs patch updates automatically; minor and major updates wait for an explicit apply.",
	})
	Register(Definition{
		Key: KeyAgentUpdateHeldNodes, Category: CategoryObservability, Kind: KindStringList, Default: "",
		Env: "APP_NODE_AGENT_UPDATE_HELD_NODES", Apply: ApplyLive,
		Title:       "Agent update held nodes",
		Description: "Node ids whose agents never update automatically. Managed with levelrail nodes agent-update hold and unhold. An operator can still update anyway.",
	})
	Register(Definition{
		Key: KeyAgentUpdateForceNodes, Category: CategoryObservability, Kind: KindStringList, Default: "",
		Env: "APP_NODE_AGENT_UPDATE_FORCE_NODES", Apply: ApplyLive,
		Title:       "Agent update anyway queue",
		Description: "Node ids with a one-time Update anyway request. Each entry is cleared when that agent picks it up; it never turns on automatic updates.",
	})
	Register(Definition{
		Key: KeyAgentFenceRequired, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "APP_AGENT_FENCE_REQUIRED", Apply: ApplyLive,
		Title:       "Require fence tokens on agents",
		Description: "When on, stop, remove and cleanup requests are refused for nodes whose agent does not understand fence tokens, so a paused node can never replay a stale action. Off keeps mixed-version fleets working; turn it on once every agent is updated.",
	})
	Register(Definition{
		Key: KeyNodeDrainStallSeconds, Category: CategoryObservability, Kind: KindInt, Default: "600",
		Env: "APP_NODE_DRAIN_STALL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(30, 86400),
		Title:       "Node drain stall seconds",
		Description: "Seconds a running node drain may go without progress before it is shown as stalled, from 30 to 86400.",
	})
}
