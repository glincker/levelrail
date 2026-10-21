package selfupgrade

import "strings"

// Failure causes, the stable ids the dashboard and CLI key their copy on.
const (
	CauseDiskFull       = "disk_full"
	CauseNetwork        = "network"
	CauseChecksum       = "checksum_mismatch"
	CauseSignature      = "signature_invalid"
	CauseBinaryUnusable = "binary_unusable"
	CauseBackupFailed   = "backup_failed"
	CauseMigration      = "migration_incompatible"
	CauseServiceControl = "service_control"
	CauseSwapFailed     = "swap_failed"
	CauseNotHealthy     = "not_healthy"
	CauseGuardRefused   = "guard_refused"
	CauseUnknown        = "unknown"
)

// Primary fix actions.
const (
	FixActionCleanup  = "cleanup"
	FixActionRetry    = "retry"
	FixActionRollback = "rollback"
	FixActionNone     = "none"
)

// DiskReclaimFixID is the fixit fix a disk_full failure leads with.
const DiskReclaimFixID = "disk.reclaim"

// StepFailure is the typed reading of one failed upgrade step.
type StepFailure struct {
	Cause string `json:"cause"`
	// Primary is the one button to lead with: cleanup, retry, rollback or none.
	Primary string `json:"primary"`
	// FixID is the fixit fix behind a cleanup primary.
	FixID string `json:"fix_id,omitempty"`
	// Retryable and Rollbackable say which buttons make sense.
	Retryable    bool `json:"retryable"`
	Rollbackable bool `json:"rollbackable"`
	// CLI is the host-side equivalent, shown collapsed.
	CLI string `json:"cli,omitempty"`
}

type failureRule struct {
	step     string
	contains []string
	out      StepFailure
}

var failureRules = []failureRule{
	{"", []string{"no space left", "disk full", "not enough free", "insufficient disk", "enospc"},
		StepFailure{Cause: CauseDiskFull, Primary: FixActionCleanup, FixID: DiskReclaimFixID, Retryable: true}},
	{"", []string{"timeout", "timed out", "connection refused", "no such host", "dial tcp", "tls handshake"},
		StepFailure{Cause: CauseNetwork, Primary: FixActionRetry, Retryable: true}},
	{StepGuard, nil, StepFailure{Cause: CauseGuardRefused, Primary: FixActionNone}},
	{StepDownload, nil, StepFailure{Cause: CauseNetwork, Primary: FixActionRetry, Retryable: true}},
	{StepChecksum, nil, StepFailure{Cause: CauseChecksum, Primary: FixActionRetry, Retryable: true}},
	{StepSignature, nil, StepFailure{Cause: CauseSignature, Primary: FixActionNone}},
	{StepProbe, nil, StepFailure{Cause: CauseBinaryUnusable, Primary: FixActionRetry, Retryable: true}},
	{StepBackup, nil, StepFailure{Cause: CauseBackupFailed, Primary: FixActionRetry, Retryable: true}},
	{StepMigrationCheck, nil, StepFailure{Cause: CauseMigration, Primary: FixActionRollback, Rollbackable: true}},
	{StepStop, nil, StepFailure{Cause: CauseServiceControl, Primary: FixActionRetry, Retryable: true}},
	{StepStart, nil, StepFailure{Cause: CauseServiceControl, Primary: FixActionRollback, Retryable: true, Rollbackable: true}},
	{StepSwap, nil, StepFailure{Cause: CauseSwapFailed, Primary: FixActionRetry, Retryable: true, Rollbackable: true}},
	{StepHealth, nil, StepFailure{Cause: CauseNotHealthy, Primary: FixActionRollback, Retryable: true, Rollbackable: true}},
}

// ClassifyFailure maps a failed step and its reason to a typed cause and fix.
// Message matches win over step defaults, so a full disk is cleanup anywhere.
func ClassifyFailure(step, reason string) StepFailure {
	low := strings.ToLower(reason)
	for _, r := range failureRules {
		for _, c := range r.contains {
			if strings.Contains(low, c) {
				return withCLI(r.out)
			}
		}
	}
	for _, r := range failureRules {
		if len(r.contains) == 0 && r.step == step {
			return withCLI(r.out)
		}
	}
	return withCLI(StepFailure{Cause: CauseUnknown, Primary: FixActionRetry, Retryable: true})
}

func withCLI(f StepFailure) StepFailure {
	switch {
	case f.Primary == FixActionRollback:
		f.CLI = "upgrade rollback"
	case f.Retryable:
		f.CLI = "upgrade retry"
	}
	return f
}
