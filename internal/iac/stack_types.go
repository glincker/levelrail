package iac

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Contract types for stacks (ADR 042). The store and the stack API share
// these so names and rules stay identical; no persistence lives here.

// KindStack is the optional stack document. It is not in Kinds because it
// configures the apply instead of describing a resource.
const KindStack Kind = "Stack"

// maxStackNameLen matches the legacy source limit so a stack name is always a
// valid managed-by source.
const maxStackNameLen = 48

var stackNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Sentinel errors the store and API map to 409 or 422 responses.
var (
	ErrStackPinned     = errors.New("stack is pinned; only a rollback to the pinned revision may apply")
	ErrStackLocked     = errors.New("stack is locked by another apply")
	ErrLedgerConflict  = errors.New("resource is owned by another stack")
	ErrPruneNotAllowed = errors.New("prune needs both the manifest request and the stack allow_prune setting")
)

// ValidateStackName checks the stack name shape.
func ValidateStackName(name string) error {
	if name == "" {
		return errors.New("stack name is required")
	}
	if len(name) > maxStackNameLen {
		return fmt.Errorf("stack name may be at most %d characters", maxStackNameLen)
	}
	if !stackNameRe.MatchString(name) {
		return errors.New("stack name must start with a lowercase letter and hold only lowercase letters, digits and hyphens")
	}
	return nil
}

// DriftPolicy says what happens when live state diverges from the stack.
type DriftPolicy string

// Drift policies; revert is deliberately not offered.
const (
	DriftWarn DriftPolicy = "warn"
	DriftLock DriftPolicy = "lock"
)

// DefaultDriftPolicy is warn so hand edits keep working until an owner opts in.
const DefaultDriftPolicy = DriftWarn

// ParseDriftPolicy maps an empty value to the default and rejects unknown ones.
func ParseDriftPolicy(s string) (DriftPolicy, error) {
	switch p := DriftPolicy(strings.TrimSpace(s)); p {
	case "":
		return DefaultDriftPolicy, nil
	case DriftWarn, DriftLock:
		return p, nil
	default:
		return "", fmt.Errorf("drift must be %q or %q, got %q", DriftWarn, DriftLock, s)
	}
}

// StackSpec is the spec of a kind: Stack document.
type StackSpec struct {
	Project     string      `json:"project" yaml:"project"`
	Environment string      `json:"environment,omitempty" yaml:"environment,omitempty"`
	Prune       bool        `json:"prune,omitempty" yaml:"prune,omitempty"`
	Drift       DriftPolicy `json:"drift,omitempty" yaml:"drift,omitempty"`
}

// Validate checks the spec and fills the default drift policy.
func (s *StackSpec) Validate() error {
	if strings.TrimSpace(s.Project) == "" {
		return errors.New("stack spec.project is required")
	}
	p, err := ParseDriftPolicy(string(s.Drift))
	if err != nil {
		return err
	}
	s.Drift = p
	return nil
}

// Stack is the server-side stack object. Settings here are admin-owned; a
// manifest can request but never grant them.
type Stack struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	ProjectID       string      `json:"project_id"`
	EnvironmentID   string      `json:"environment_id,omitempty"`
	AllowPrune      bool        `json:"allow_prune"`
	DriftPolicy     DriftPolicy `json:"drift_policy"`
	PinnedRevision  int         `json:"pinned_revision,omitempty"` // 0 means not pinned
	RequireApproval bool        `json:"require_approval"`
	CreatedAt       time.Time   `json:"created_at"`
	CreatedBy       string      `json:"created_by"`
}

// Validate checks the fields a store must never persist wrong.
func (s *Stack) Validate() error {
	if err := ValidateStackName(s.Name); err != nil {
		return err
	}
	if s.ProjectID == "" {
		return errors.New("stack project is required")
	}
	if _, err := ParseDriftPolicy(string(s.DriftPolicy)); err != nil {
		return err
	}
	if s.PinnedRevision < 0 {
		return errors.New("pinned revision must be positive")
	}
	return nil
}

// PruneAllowed needs both sides so a manifest cannot grant itself prune.
func (s *Stack) PruneAllowed(manifestPrune bool) error {
	if manifestPrune && s.AllowPrune {
		return nil
	}
	return ErrPruneNotAllowed
}

// CheckPin returns ErrStackPinned unless the apply is a rollback to the pin.
// rollbackOf is 0 for an ordinary apply.
func (s *Stack) CheckPin(rollbackOf int) error {
	if s.PinnedRevision == 0 || rollbackOf == s.PinnedRevision {
		return nil
	}
	return ErrStackPinned
}

// RevisionStatus is the lifecycle of one stack apply.
type RevisionStatus string

// Revision statuses, in lifecycle order.
const (
	RevisionPendingApproval RevisionStatus = "pending_approval"
	RevisionApplying        RevisionStatus = "applying"
	RevisionApplied         RevisionStatus = "applied"
	RevisionPartial         RevisionStatus = "partial"
	RevisionFailed          RevisionStatus = "failed"
	RevisionSuperseded      RevisionStatus = "superseded"
)

// Valid reports whether s is a known status.
func (s RevisionStatus) Valid() bool {
	_, ok := revisionNext[s]
	return ok
}

// Terminal statuses never change again, except applied becoming superseded.
func (s RevisionStatus) Terminal() bool {
	return s == RevisionApplied || s == RevisionPartial || s == RevisionFailed || s == RevisionSuperseded
}

// revisionNext is the allowed transition table; the store enforces it so a
// crashed apply cannot jump a revision backwards.
var revisionNext = map[RevisionStatus][]RevisionStatus{
	RevisionPendingApproval: {RevisionApplying, RevisionFailed},
	RevisionApplying:        {RevisionApplied, RevisionPartial, RevisionFailed},
	RevisionApplied:         {RevisionSuperseded},
	RevisionPartial:         {RevisionSuperseded},
	RevisionFailed:          {},
	RevisionSuperseded:      {},
}

// CanTransition reports whether a revision may move from s to next.
func (s RevisionStatus) CanTransition(next RevisionStatus) bool {
	for _, n := range revisionNext[s] {
		if n == next {
			return true
		}
	}
	return false
}

// GitMeta links a revision to the commit and CI run that produced it.
type GitMeta struct {
	Repo     string `json:"git_repo,omitempty"`
	Ref      string `json:"git_ref,omitempty"`
	SHA      string `json:"git_sha,omitempty"`
	CIRunURL string `json:"ci_run_url,omitempty"`
}

// Revision is one numbered apply of a stack. RenderedDocs holds documents
// after env substitution only; secret values are never part of it.
type Revision struct {
	StackID        string         `json:"stack_id"`
	Number         int            `json:"number"`
	Status         RevisionStatus `json:"status"`
	PlanHash       string         `json:"plan_hash"`
	PlanJSON       []byte         `json:"plan,omitempty"`
	RenderedDocs   []byte         `json:"rendered_docs,omitempty"`
	RollbackOf     int            `json:"rollback_of,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Actor          string         `json:"actor"`
	TokenID        string         `json:"token_id,omitempty"`
	Git            GitMeta        `json:"git"`
	CreatedAt      time.Time      `json:"created_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	ResultJSON     []byte         `json:"result,omitempty"`
}

// Validate checks a revision before it is first stored.
func (r *Revision) Validate() error {
	if r.StackID == "" {
		return errors.New("revision stack is required")
	}
	if r.Number < 1 {
		return errors.New("revision number starts at 1")
	}
	if !r.Status.Valid() {
		return fmt.Errorf("unknown revision status %q", r.Status)
	}
	if r.RollbackOf < 0 || r.RollbackOf >= r.Number {
		return errors.New("rollback_of must name an earlier revision")
	}
	return nil
}

// LeakedSecret returns the name of a secret whose value appears in docs, so a
// revision can be refused before it is stored. Empty values are skipped.
func LeakedSecret(docs []byte, secrets map[string]string) (string, bool) {
	text := string(docs)
	for _, name := range sortedKeys(secrets) {
		if v := secrets[name]; v != "" && strings.Contains(text, v) {
			return name, true
		}
	}
	return "", false
}

// LedgerEntry records the single stack that owns a resource. Key is the
// resourceKey (kind/scope/name), unique per kind across all stacks.
type LedgerEntry struct {
	StackID       string `json:"stack_id"`
	Kind          Kind   `json:"kind"`
	Key           string `json:"key"`
	LiveID        string `json:"live_id"`
	SinceRevision int    `json:"since_revision"`
}

// StackLock is the per-stack lease. Its length comes from APP_STACK_LOCK_TTL.
type StackLock struct {
	StackID   string    `json:"stack_id"`
	Holder    string    `json:"holder"`
	Revision  int       `json:"revision"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired reports whether another holder may take the lease over.
func (l StackLock) Expired(now time.Time) bool { return !now.Before(l.ExpiresAt) }

// StackDeleteMode picks what deleting a stack does to its resources.
type StackDeleteMode string

const (
	// StackForget releases the ledger and leaves every resource in place.
	StackForget StackDeleteMode = "forget"
	// StackDestroy deletes ledger resources except databases; never from CI.
	StackDestroy StackDeleteMode = "destroy"
)

// ParseStackDeleteMode defaults to forget, the mode that cannot lose data.
func ParseStackDeleteMode(s string) (StackDeleteMode, error) {
	switch m := StackDeleteMode(strings.TrimSpace(s)); m {
	case "":
		return StackForget, nil
	case StackForget, StackDestroy:
		return m, nil
	default:
		return "", fmt.Errorf("mode must be %q or %q, got %q", StackForget, StackDestroy, s)
	}
}

// StackScope is the apply context a stack handler attaches to loopback
// sub-requests. IAM trusts only this server-side value, never a header.
type StackScope struct {
	StackID   string
	StackName string
	Revision  int
}

type stackScopeKey struct{}

// WithStackScope returns ctx carrying the stack apply scope.
func WithStackScope(ctx context.Context, s StackScope) context.Context {
	return context.WithValue(ctx, stackScopeKey{}, s)
}

// StackScopeFrom returns the stack apply scope, if the request runs inside one.
func StackScopeFrom(ctx context.Context) (StackScope, bool) {
	s, ok := ctx.Value(stackScopeKey{}).(StackScope)
	return s, ok && s.StackID != ""
}
