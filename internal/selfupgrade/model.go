// Package selfupgrade replaces the control plane binary with a verified
// newer release and rolls binary and data back if it does not become healthy.
// It runs from a host command, never inside the process being replaced.
package selfupgrade

import (
	"encoding/json"
	"time"
)

// Step names, in the order Apply runs them.
const (
	StepGuard          = "guard"
	StepDownload       = "download"
	StepChecksum       = "verify_checksum"
	StepSignature      = "verify_signature"
	StepProbe          = "probe_binary"
	StepBackup         = "backup"
	StepMigrationCheck = "migration_check"
	StepStop           = "stop"
	StepSwap           = "swap"
	StepStart          = "start"
	StepHealth         = "health"
	StepRollback       = "rollback"
)

// Step statuses.
const (
	StatusOK      = "ok"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Attempt outcomes. Running is only ever seen in a journal.
const (
	OutcomeRunning    = "running"
	OutcomeSucceeded  = "succeeded"
	OutcomeRolledBack = "rolled_back"
	OutcomeRefused    = "refused"
	OutcomeFailed     = "failed"
)

// StepRecord is one step's result in an attempt's timeline.
type StepRecord struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms"`
}

// Attempt is one upgrade run. The unexported-to-the-database fields at the end
// (paths) only live in the journal so a restart in the middle can recover.
type Attempt struct {
	ID          string       `json:"id"`
	FromVersion string       `json:"from_version"`
	ToVersion   string       `json:"to_version"`
	FromSchema  int          `json:"from_schema"`
	ToSchema    int          `json:"to_schema"`
	Initiator   string       `json:"initiator"`
	Outcome     string       `json:"outcome"`
	FailedStep  string       `json:"failed_step,omitempty"`
	Error       string       `json:"error,omitempty"`
	BackupName  string       `json:"backup_name,omitempty"`
	Acked       []string     `json:"acked,omitempty"`
	Steps       []StepRecord `json:"steps"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at,omitempty"`

	PrevBinary    string `json:"prev_binary,omitempty"`
	SnapshotPath  string `json:"snapshot_path,omitempty"`
	SwappedBinary bool   `json:"swapped_binary,omitempty"`
}

// lastStep names the newest step on the timeline.
func (a Attempt) lastStep() string {
	if len(a.Steps) == 0 {
		return ""
	}
	return a.Steps[len(a.Steps)-1].Name
}

// StepsJSON encodes the timeline for the database column.
func (a Attempt) StepsJSON() string {
	raw, err := json.Marshal(a.Steps)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// AckedJSON encodes the acknowledged breaking-change ids for the database.
func (a Attempt) AckedJSON() string {
	raw, err := json.Marshal(a.Acked)
	if err != nil || a.Acked == nil {
		return "[]"
	}
	return string(raw)
}
