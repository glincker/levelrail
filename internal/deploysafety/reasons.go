// Package deploysafety decides whether a new release may take traffic and
// whether a live one must be rolled back, and holds the shared reason strings.
package deploysafety

// Phases of a deploy's gate timeline.
const (
	PhaseReadiness = "readiness"
	PhaseStability = "stability"
	PhaseCutover   = "cutover"
	PhaseDrain     = "drain"
	PhaseWatch     = "watch"
	PhaseRollback  = "rollback"
	PhasePin       = "pin"
)

// Statuses a gate event can carry.
const (
	StatusStarted = "started"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusInfo    = "info"
	// StatusStopped marks a watch an operator ended early.
	StatusStopped = "stopped"
)

// Watch modes: off skips the watch, notify alerts on a trip, auto also
// rolls back.
const (
	WatchModeOff    = "off"
	WatchModeNotify = "notify"
	WatchModeAuto   = "auto"
)

// Reasons a deploy fails its gate or a live release is rolled back. These
// land in deploy_attempts.reason, gate events and reconcile conditions.
const (
	ReasonReadinessFailed          = "ReadinessFailed"
	ReasonOOMKilledDuringReadiness = "OOMKilledDuringReadiness"
	ReasonExitedDuringReadiness    = "ExitedDuringReadiness"
	ReasonOOMKilledDuringWindow    = "OOMKilledDuringStableWindow"
	ReasonExitedDuringWindow       = "ExitedDuringStableWindow"
	ReasonRestartedDuringWindow    = "RestartedDuringStableWindow"
	ReasonGateTimeout              = "StabilityGateTimeout"
	ReasonWatchCrashloop           = "PostDeployCrashloop"
	ReasonWatchOOMKilled           = "PostDeployOOMKilled"
	ReasonWatchExited              = "PostDeployExited"
	ReasonWatchErrorRate           = "PostDeployErrorRate"
	ReasonWatchLatency             = "PostDeployLatencyRegression"
	ReasonWatchStopped             = "WatchStopped"
	ReasonAutoRolledBack           = "AutoRolledBack"
	ReasonNoRollbackTarget         = "NoRollbackTarget"
	ReasonOperatorRollback         = "OperatorRollback"
)

// GateFailureReasons is every reason that means a release never took traffic.
var GateFailureReasons = []string{
	ReasonReadinessFailed, ReasonOOMKilledDuringReadiness, ReasonExitedDuringReadiness,
	ReasonOOMKilledDuringWindow, ReasonExitedDuringWindow, ReasonRestartedDuringWindow, ReasonGateTimeout,
}

// WatchTripReasons is every reason the post-deploy watch can trip on.
var WatchTripReasons = []string{ReasonWatchCrashloop, ReasonWatchOOMKilled, ReasonWatchExited, ReasonWatchErrorRate, ReasonWatchLatency}
