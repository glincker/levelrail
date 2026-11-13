package cloudiac

import (
	"context"
	"errors"
)

// Step names one engine operation.
type Step string

// The steps an engine can run.
const (
	StepPlan    Step = "plan"
	StepApply   Step = "apply"
	StepDestroy Step = "destroy"
)

// ModuleSpec is a rendered module the engine runs, never raw user input.
type ModuleSpec struct {
	Name      string
	Provider  string
	Files     map[string][]byte
	Variables map[string]string
}

// CredentialHandle refers to a short-lived credential for one step. The
// engine resolves it at run time so secrets never sit in specs or results.
type CredentialHandle struct {
	ID        string
	ExpiresAt int64
}

// Change is one redacted resource change reported by a step.
type Change struct {
	Address string
	Action  string
}

// Result is the typed outcome of a step. Output is already redacted.
type Result struct {
	Step    Step
	Changes []Change
	Outputs map[string]string
	Log     string
	// PlanHash names the saved plan a plan produced or an apply ran.
	PlanHash string
	// StateVersion is the stored state version after the step.
	StateVersion int64
	// Partial means the apply failed but the state it reached was stored.
	Partial bool
	// Diverged means the new state could not be stored under the fence.
	Diverged bool
}

// ErrorKind classifies a step failure so callers can react without parsing text.
type ErrorKind string

// The failure classes an engine reports.
const (
	ErrInvalidSpec ErrorKind = "invalid_spec"
	ErrCredential  ErrorKind = "credential"
	ErrProvider    ErrorKind = "provider"
	ErrCanceled    ErrorKind = "canceled"
	ErrInternal    ErrorKind = "internal"
)

// Error is a step failure with a redacted message.
type Error struct {
	Kind    ErrorKind
	Step    Step
	Message string
	Err     error
}

func (e *Error) Error() string { return string(e.Step) + ": " + string(e.Kind) + ": " + e.Message }

func (e *Error) Unwrap() error { return e.Err }

// ErrNotConfigured is returned when no engine implementation is wired in.
var ErrNotConfigured = errors.New("cloudiac: no engine configured")

// Engine runs provisioning steps. Other engines can implement it later.
type Engine interface {
	Plan(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (Result, error)
	Apply(ctx context.Context, req ApplyRequest) (Result, error)
	Destroy(ctx context.Context, req ApplyRequest) (Result, error)
}
