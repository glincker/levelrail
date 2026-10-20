package rollback

import (
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// Step codes in a Plan. The dashboard and CLI translate them.
const (
	StepBackup        = "backup"
	StepFetch         = "fetch"
	StepVerify        = "verify"
	StepStop          = "stop"
	StepRestore       = "restore_backup"
	StepSwap          = "swap_binary"
	StepStart         = "start"
	StepHealth        = "verify_health"
	StepAutoRecover   = "auto_recover"
	warnDataLoss      = "data_loss"
	warnUnknownSchema = "unknown_schema"
	warnForward       = "forward_migration"
	warnNoBackup      = "no_compatible_backup"
	warnDev           = "dev_build"
)

// Schema sources for a target.
const (
	SourceRetained = "retained"
	SourceManifest = "manifest"
	SourceUnknown  = "unknown"
)

// Target describes the release being rolled back to.
type Target struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	SchemaSource  string `json:"schema_source"`
	Retained      bool   `json:"retained"`
	SHA256        string `json:"sha256,omitempty"`
}

// BackupOption is a database backup a restore could use.
type BackupOption struct {
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion int       `json:"schema_version"`
	Compatible    bool      `json:"compatible"`
}

// Plan is everything an operator needs to decide on a rollback.
type Plan struct {
	CurrentVersion       string                `json:"current_version"`
	CurrentSchemaVersion int                   `json:"current_schema_version"`
	Target               Target                `json:"target"`
	Verdict              upgrade.SchemaVerdict `json:"verdict"`
	Steps                []string              `json:"steps"`
	Warnings             []string              `json:"warnings"`
	DowntimeSeconds      int                   `json:"downtime_seconds"`
	RestoreRequired      bool                  `json:"restore_required"`
	Backups              []BackupOption        `json:"backups"`
	LossSince            *time.Time            `json:"data_loss_since,omitempty"`
	Command              string                `json:"command"`
	RestoreCommand       string                `json:"restore_command,omitempty"`
}

// PlanInput is the facts BuildPlan needs, gathered by the caller.
type PlanInput struct {
	CurrentVersion       string
	CurrentSchemaVersion int
	DBSizeBytes          int64
	Target               Target
	Backups              []BackupOption
	ProgramName          string
}

const (
	baseDowntimeSeconds = 15
	restoreBytesPerSec  = 50 << 20
)

// BuildPlan derives the verdict, steps, warnings and downtime for a rollback.
// It is pure: callers supply the schema versions and backup list.
func BuildPlan(in PlanInput) Plan {
	p := Plan{
		CurrentVersion:       in.CurrentVersion,
		CurrentSchemaVersion: in.CurrentSchemaVersion,
		Target:               in.Target,
		Verdict:              upgrade.VerdictFor(in.CurrentSchemaVersion, in.Target.SchemaVersion),
		Steps:                []string{StepBackup},
		Warnings:             []string{},
		DowntimeSeconds:      baseDowntimeSeconds,
	}
	if !in.Target.Retained {
		p.Steps = append(p.Steps, StepFetch)
	}
	p.Steps = append(p.Steps, StepVerify, StepStop)
	p.Command = "sudo " + in.ProgramName + " rollback --to " + in.Target.Version

	p.Backups = markCompatible(in.Backups, in.Target.SchemaVersion)
	switch p.Verdict {
	case upgrade.VerdictRestoreRequired:
		p.RestoreRequired = true
		p.Steps = append(p.Steps, StepRestore)
		p.Warnings = append(p.Warnings, warnDataLoss)
		p.DowntimeSeconds += int(in.DBSizeBytes/restoreBytesPerSec) + 1
		if best, ok := newestCompatible(p.Backups); ok {
			t := best.CreatedAt
			p.LossSince = &t
			p.RestoreCommand = p.Command + " --restore-backup " + best.Name + " --confirm-data-loss " + best.Name
		} else {
			p.Warnings = append(p.Warnings, warnNoBackup)
		}
	case upgrade.VerdictUnknown:
		p.Warnings = append(p.Warnings, warnUnknownSchema)
	case upgrade.VerdictForward:
		p.Warnings = append(p.Warnings, warnForward)
	}
	if in.CurrentVersion == "dev" {
		p.Warnings = append(p.Warnings, warnDev)
	}
	p.Steps = append(p.Steps, StepSwap, StepStart, StepHealth, StepAutoRecover)
	return p
}

func markCompatible(in []BackupOption, targetSchema int) []BackupOption {
	out := make([]BackupOption, 0, len(in))
	for _, b := range in {
		b.Compatible = targetSchema >= 0 && b.SchemaVersion >= 0 && b.SchemaVersion <= targetSchema
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func newestCompatible(bs []BackupOption) (BackupOption, bool) {
	for _, b := range bs {
		if b.Compatible {
			return b, true
		}
	}
	return BackupOption{}, false
}
