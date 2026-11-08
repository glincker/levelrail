package runtimesettings

import "time"

const (
	// KeyNodesPatchPreviewTTL is how long a patch preview token stays valid for an apply.
	KeyNodesPatchPreviewTTL = "nodes.patch_preview_ttl"
	// KeyNodesPatchApplyTimeout bounds one package upgrade run on a node.
	KeyNodesPatchApplyTimeout = "nodes.patch_apply_timeout"
	// KeyNodesPatchOutputTailLines is how many lines of package manager output a run keeps.
	KeyNodesPatchOutputTailLines = "nodes.patch_output_tail_lines"
	// KeyNodesPatchRunHistory is how many patch runs per node stay listed.
	KeyNodesPatchRunHistory = "nodes.patch_run_history"
	// KeyNodesPatchPollInterval is how often a detached upgrade unit's state is read.
	KeyNodesPatchPollInterval = "nodes.patch_poll_interval"
	// KeyNodesPatchAdoptLimit caps how many leftover upgrade units a restarted control plane adopts.
	KeyNodesPatchAdoptLimit = "nodes.patch_adopt_limit"
)

func init() {
	Register(Definition{
		Key: KeyNodesPatchPreviewTTL, Category: CategorySecurity, Kind: KindDuration, Default: "10m",
		Env: "APP_NODES_PATCH_PREVIEW_TTL", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, time.Hour),
		Title:       "Server patch preview lifetime",
		Description: "How long a patch preview stays valid. An apply must quote a preview younger than this, and the package list must still match it.",
	})
	Register(Definition{
		Key: KeyNodesPatchApplyTimeout, Category: CategorySecurity, Kind: KindDuration, Default: "30m",
		Env: "APP_NODES_PATCH_APPLY_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, 4*time.Hour),
		Title:       "Server patch apply timeout",
		Description: "How long the control plane waits for one package upgrade. On timeout the upgrade is left running and the run is recorded as timed_out; it is never killed.",
	})
	Register(Definition{
		Key: KeyNodesPatchOutputTailLines, Category: CategoryObservability, Kind: KindInt, Default: "200",
		Env: "APP_NODES_PATCH_OUTPUT_TAIL_LINES", Apply: ApplyLive, CheckValue: intRangeCheck(10, 5000),
		Title:       "Server patch output lines kept",
		Description: "How many trailing lines of package manager output each patch run keeps, with secret looking values masked.",
	})
	Register(Definition{
		Key: KeyNodesPatchRunHistory, Category: CategoryRetention, Kind: KindInt, Default: "20",
		Env: "APP_NODES_PATCH_RUN_HISTORY", Apply: ApplyLive, CheckValue: intRangeCheck(1, 500),
		Title:       "Server patch runs kept per node",
		Description: "How many recent patch runs per node the control plane lists. History lives in memory and resets on restart; the audit log keeps every run.",
	})
	Register(Definition{
		Key: KeyNodesPatchPollInterval, Category: CategoryObservability, Kind: KindDuration, Default: "2s",
		Env: "APP_NODES_PATCH_POLL_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, time.Minute),
		Title:       "Server patch status poll interval",
		Description: "How often the control plane reads the state of a package upgrade running as its own systemd unit. Clamped to 500ms through 60s.",
	})
	Register(Definition{
		Key: KeyNodesPatchAdoptLimit, Category: CategorySecurity, Kind: KindInt, Default: "20",
		Env: "APP_NODES_PATCH_ADOPT_LIMIT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 200),
		Title:       "Server patch units adopted after a restart",
		Description: "How many package upgrade units left by a previous control plane process are picked up again, with their result recorded and audited. A unit still running keeps the node locked until it ends.",
	})
}
