// Package cleanup is the retention center: a registry of cleanup targets, a
// scheduler that runs them under per-target modes, and a run history.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Category groups targets in the dashboard.
type Category string

// Target categories.
const (
	CategoryAuth          Category = "auth"
	CategoryAudit         Category = "audit"
	CategoryDeploys       Category = "deploys"
	CategoryDocker        Category = "docker"
	CategoryData          Category = "data"
	CategoryObservability Category = "observability"
	CategoryMessaging     Category = "messaging"
	CategoryOther         Category = "other"
)

// Origin says whether a target replaces an existing sweeper or covers data that was never bounded.
type Origin string

// Target origins.
const (
	OriginExisting Origin = "existing"
	OriginNew      Origin = "new"
)

// Mode is what the scheduler does with a target.
type Mode string

// Target modes.
const (
	ModeOff     Mode = "off"
	ModePreview Mode = "preview"
	ModeOn      Mode = "on"
)

// Trigger records what started a run.
type Trigger string

// Run triggers.
const (
	TriggerScheduled Trigger = "scheduled"
	TriggerManual    Trigger = "manual"
	TriggerCLI       Trigger = "cli"
	TriggerAPI       Trigger = "api"
)

// Event types emitted through the event webhook mechanism.
const (
	EventRun    = "cleanup.run"
	EventFailed = "cleanup.failed"
)

// AuditActionRun is the audit action written for every run, dry runs included.
const AuditActionRun = "cleanup_run"

// Errors the center returns; handlers map them to HTTP statuses.
var (
	ErrUnknownTarget = errors.New("cleanup: unknown target")
	ErrBusy          = errors.New("cleanup: this target is already running")
	ErrReadOnly      = errors.New("cleanup: this target is managed elsewhere")
	ErrNoRetention   = errors.New("cleanup: retention is set to keep forever, pass days to clean anyway")
	ErrInvalidDays   = errors.New("cleanup: days must be a positive number")
)

// FloorError means a requested window is shorter than the target's minimum.
type FloorError struct {
	Key   string
	Floor int
}

func (e *FloorError) Error() string {
	return fmt.Sprintf("retention cannot be shorter than %d days", e.Floor)
}

// Usage is what a target holds now. Bytes is 0 when the store cannot report size.
type Usage struct {
	Rows   int64      `json:"rows"`
	Bytes  int64      `json:"bytes"`
	Oldest *time.Time `json:"oldest"`
}

// Impact is what a run with a given window would remove.
type Impact struct {
	Rows  int64 `json:"rows"`
	Bytes int64 `json:"bytes"`
}

// Result is what a run removed. More says the row cap stopped it early.
type Result struct {
	Rows  int64
	Bytes int64
	More  bool
	Err   error
}

// RunOptions say how one run behaves.
type RunOptions struct {
	DryRun  bool
	Trigger Trigger
	Actor   string
	// ActorType, ActorID and RemoteAddr fill the audit row; empty means a system actor.
	ActorType  string
	ActorID    string
	RemoteAddr string
	// MaxRows caps one run; Slice is the time span of one delete batch. The center fills both from its limits.
	MaxRows int64
	Slice   time.Duration
}

// Meta is a target's static description.
type Meta struct {
	// ID is dotted, for example "audit.log".
	ID       string
	Category Category
	// Title and Description are English fallbacks for the i18n keys.
	TitleKey, Title      string
	DescKey, Description string
	// PolicySetting is the runtime setting holding the retention days.
	PolicySetting string
	// Floor is the shortest allowed retention in days, 0 for none.
	Floor       int
	Destructive bool
	Origin      Origin
	// Presets lists the day counts the dashboard offers; empty means the standard presets.
	Presets []int
	// ReadOnly marks a target whose policy and runs live elsewhere (the janitor); Link says where.
	ReadOnly bool
	Link     string
}

// Target is one kind of data the center can clean.
type Target interface {
	Meta() Meta
	Measure(ctx context.Context) (Usage, error)
	Preview(ctx context.Context, days int) (Impact, error)
	// Run removes data older than days. With opts.DryRun it must not write.
	Run(ctx context.Context, days int, opts RunOptions) Result
}

// Managed is implemented by read-through targets that report their own state.
type Managed interface {
	State(ctx context.Context) (ManagedState, error)
}

// ManagedState is the policy a read-through target shows.
type ManagedState struct {
	Days   int
	Mode   Mode
	DryRun bool
}
