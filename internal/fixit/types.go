// Package fixit turns failing readiness and diagnostic checks into typed,
// previewable, verifiable fixes with attempt memory and loop protection.
package fixit

import (
	"context"
	"errors"
	"time"
)

// Kind is how much of a fix the control plane can do itself.
type Kind string

// Kind values: the control plane applies it, shows an exact command, or only explains.
const (
	KindAuto   Kind = "auto"
	KindGuided Kind = "guided"
	KindManual Kind = "manual"
)

// Risk is the worst thing applying a fix can do.
type Risk string

// Risk values, from harmless to externally visible.
const (
	RiskSafe     Risk = "safe"
	RiskRestarts Risk = "restarts"
	RiskExposes  Risk = "exposes"
)

// OutcomeStatus is the result of one apply or re-check.
type OutcomeStatus string

// Outcome values for an apply or re-check.
const (
	OutcomeFixed         OutcomeStatus = "fixed"
	OutcomeStillFailing  OutcomeStatus = "still_failing"
	OutcomeCannotFixHere OutcomeStatus = "cannot_fix_here"
	OutcomeSkipped       OutcomeStatus = "skipped"
)

// State is where a fix stands for its subject right now.
type State string

// State values for a fix against its subject.
const (
	StateHealthy     State = "healthy"
	StateOffer       State = "offer"
	StateDemoted     State = "demoted"
	StateRateLimited State = "rate_limited"
)

// ErrCannotFixHere is returned by a fix's Run when this host cannot be
// repaired from inside the control plane.
var ErrCannotFixHere = errors.New("fixit: cannot be fixed from here")

// ErrUnknownFix is returned for an id that is not registered.
var ErrUnknownFix = errors.New("fixit: unknown fix")

// Change is one thing an apply would do, shown before it happens.
type Change struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// Option is a candidate value for a fix parameter.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Param is the one input a fix may ask for.
type Param struct {
	Name    string   `json:"name"`
	Options []Option `json:"options"`
	Default string   `json:"default,omitempty"`
	Free    bool     `json:"free"`
}

// Applicability is what a fix's Detect found out about the host right now.
// It doubles as the re-verification result after an apply.
type Applicability struct {
	// Failing is true while the originating check does not pass.
	Failing bool `json:"failing"`
	// Detail is the check result as a person reads it.
	Detail string `json:"detail"`
	// Signature feeds the fingerprint; empty means Detail. Set it when Detail
	// carries volatile numbers that should not re-arm a demoted fix.
	Signature string `json:"-"`
	// Kind lowers the fix's tier for this host; empty keeps Fix.Kind.
	Kind    Kind     `json:"kind,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Changes []Change `json:"changes"`
	Command string   `json:"command,omitempty"`
	Param   *Param   `json:"param,omitempty"`
}

// Input is what Run receives.
type Input struct {
	Actor  string
	Params map[string]string
}

// Fix is one registered remedy for one check.
type Fix struct {
	ID         string
	CheckID    string
	Subject    string
	Kind       Kind
	Title      string
	Why        string
	Risk       Risk
	Reversible bool
	// Needs is the API ability the caller must hold to apply.
	Needs string
	// Detect re-runs the originating check and plans the fix.
	Detect func(ctx context.Context) (Applicability, error)
	// Run performs the change; nil for guided and manual fixes.
	Run func(ctx context.Context, in Input) error
}

// Attempt is one persisted apply or re-check.
type Attempt struct {
	FixID       string        `json:"fix_id"`
	Subject     string        `json:"subject"`
	At          time.Time     `json:"at"`
	Actor       string        `json:"actor"`
	Outcome     OutcomeStatus `json:"outcome"`
	Detail      string        `json:"detail"`
	Fingerprint string        `json:"fingerprint"`
}

// Store persists attempt memory.
type Store interface {
	RecentAttempts(ctx context.Context, fixID, subject string, limit int) ([]Attempt, error)
	CountAttemptsSince(ctx context.Context, fixID string, since time.Time) (int, error)
	RecordAttempt(ctx context.Context, a Attempt) error
}

// View is a fix as the API and UI show it.
type View struct {
	ID         string        `json:"id"`
	CheckID    string        `json:"check_id"`
	Subject    string        `json:"subject"`
	Title      string        `json:"title"`
	Why        string        `json:"why"`
	Risk       Risk          `json:"risk"`
	Reversible bool          `json:"reversible"`
	Needs      string        `json:"needs"`
	State      State         `json:"state"`
	BaseKind   Kind          `json:"base_kind"`
	Kind       Kind          `json:"kind"`
	Failing    bool          `json:"failing"`
	Detail     string        `json:"detail"`
	Reason     string        `json:"reason,omitempty"`
	Changes    []Change      `json:"changes"`
	Command    string        `json:"command,omitempty"`
	Param      *Param        `json:"param,omitempty"`
	LastTried  *Attempt      `json:"last_attempt,omitempty"`
	Attempts   []Attempt     `json:"attempts"`
	Blocked    string        `json:"blocked,omitempty"`
	RetryAfter time.Duration `json:"-"`
}

// Result is the outcome of Apply.
type Result struct {
	FixID   string        `json:"fix_id"`
	Outcome OutcomeStatus `json:"outcome"`
	Detail  string        `json:"detail"`
	View    View          `json:"view"`
}
