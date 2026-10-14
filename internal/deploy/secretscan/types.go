// Package secretscan finds credentials committed to a build context or
// stored as plain environment values, and applies a per-app deploy policy.
// A finding never carries the matched value, only a short fingerprint.
package secretscan

import (
	"errors"
	"fmt"
	"time"
)

// Policy is what a scan may do to a release.
type Policy string

// Policies. Warn is the default.
const (
	PolicyOff   Policy = "off"
	PolicyWarn  Policy = "warn"
	PolicyBlock Policy = "block"
)

// Sources a finding came from.
const (
	SourceFile = "file"
	SourceEnv  = "env"
)

// Gate actions stored on a run.
const (
	ActionSkipped = "skipped"
	ActionAllow   = "allow"
	ActionWarn    = "warn"
	ActionBlock   = "block"
)

// Run statuses.
const (
	StatusComplete = "complete"
	StatusPartial  = "partial"
	StatusError    = "error"
)

// ErrInvalid marks a rejected setting or request.
var ErrInvalid = errors.New("secretscan: invalid request")

// ParsePolicy validates a policy name.
func ParsePolicy(s string) (Policy, error) {
	switch p := Policy(s); p {
	case PolicyOff, PolicyWarn, PolicyBlock:
		return p, nil
	}
	return "", fmt.Errorf("%w: policy must be off, warn or block", ErrInvalid)
}

// Finding is one suspected credential. Path is relative to the build
// context; Var is set for env findings.
type Finding struct {
	RuleID      string `json:"rule_id"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Var         string `json:"var,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Allowed     bool   `json:"allowed,omitempty"`
}

// Limits bound a scan so a huge repo cannot stall a deploy.
type Limits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxFindings   int
	Timeout       time.Duration
}

// DefaultLimits are the built-in caps, overridable through the environment.
func DefaultLimits() Limits {
	return Limits{MaxFiles: 5000, MaxFileBytes: 1 << 20, MaxTotalBytes: 64 << 20, MaxFindings: 200, Timeout: 30 * time.Second}
}

// Result is a scan's outcome. Truncated means a cap stopped the walk early.
type Result struct {
	Findings     []Finding `json:"findings"`
	FilesScanned int       `json:"files_scanned"`
	FilesSkipped int       `json:"files_skipped"`
	Truncated    bool      `json:"truncated"`
}

// Run is the persisted outcome of one deploy's scan.
type Run struct {
	AttemptID    string    `json:"deployment_id"`
	App          string    `json:"app"`
	Policy       Policy    `json:"policy"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason,omitempty"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	FilesScanned int       `json:"files_scanned"`
	Truncated    bool      `json:"truncated"`
	Findings     []Finding `json:"findings"`
	ScannedAt    time.Time `json:"scanned_at"`
}

// AllowEntry suppresses one fingerprint for an app, with who and why.
type AllowEntry struct {
	App         string    `json:"app,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Reason      string    `json:"reason"`
	Actor       string    `json:"actor"`
	CreatedAt   time.Time `json:"created_at"`
}
