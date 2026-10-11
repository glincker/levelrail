// Package cutover plans and runs a guided, reversible move of one staged
// import app: readiness rules, a persisted state machine, DNS undo data.
// It has no access to the source platform by construction: the only things
// it can touch are the app here, this node's ingress and the domain's DNS.
package cutover

import "time"

// Run states.
const (
	StatePlanning   = "planning"
	StateReady      = "ready"
	StateStarting   = "starting"
	StateVerifying  = "verifying"
	StateSwitching  = "switching"
	StateLive       = "live"
	StateRolledBack = "rolled_back"
	StateFailed     = "failed"
)

// Run modes.
const (
	ModeDryRun = "dry_run"
	ModeSwitch = "switch"
)

// How a domain's traffic is switched.
const (
	MethodDNS    = "dns"
	MethodProxy  = "proxy"
	MethodManual = "manual"
	// MethodNone means the domain already resolves here, nothing to change.
	MethodNone  = "none"
	MethodMixed = "mixed"
)

// AwaitingManualDNS is what a paused run waits for: the operator's record change.
const AwaitingManualDNS = "manual_dns"

// Check statuses.
const (
	StatusPass  = "pass"
	StatusWarn  = "warn"
	StatusBlock = "block"
)

// Plan verdicts.
const (
	VerdictReady   = "ready"
	VerdictWarn    = "warnings"
	VerdictBlocked = "blocked"
)

// Step states.
const (
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Step names in the run timeline.
const (
	StepReadiness   = "readiness"
	StepRoute       = "route"
	StepHealthy     = "healthy"
	StepIngress     = "ingress_probe"
	StepDNSPreview  = "dns_preview"
	StepSwitch      = "switch"
	StepPostVerify  = "post_verify"
	StepCleanup     = "cleanup"
	StepRollback    = "rollback"
	StepManualDNS   = "manual_dns"
	StepResumeCheck = "resume_check"
)

// Check ids in a readiness plan.
const (
	CheckImage    = "image"
	CheckEnv      = "env"
	CheckDatabase = "database"
	CheckVolumes  = "volumes"
	CheckHealth   = "health"
	CheckDomains  = "domains"
	CheckState    = "state"
)

// Action kinds a fix can carry.
const (
	ActionRequest = "request"
	ActionLink    = "link"
	ActionCopy    = "copy"
)

// Action is the one primary thing a fix offers.
type Action struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	API   string `json:"api,omitempty"`
	Value string `json:"value,omitempty"`
}

// Fix is the remedy for a warning or blocking check.
type Fix struct {
	Summary string  `json:"summary"`
	Action  *Action `json:"action,omitempty"`
}

// Check is one readiness item.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fix    *Fix   `json:"fix,omitempty"`
}

// Record is one DNS record as the operator sees it.
type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl_seconds,omitempty"`
}

// DomainPlan is what switching one domain involves.
type DomainPlan struct {
	Domain string `json:"domain"`
	Method string `json:"method"`
	// Current is what the domain resolves to or the zone holds now.
	Current  []string `json:"current,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Zone     string   `json:"zone,omitempty"`
	Desired  *Record  `json:"desired,omitempty"`
	// Replace lists the provider records a DNS switch removes first.
	Replace []Record `json:"replace,omitempty"`
	Message string   `json:"message,omitempty"`
}

// Plan is the readiness checklist for one app.
type Plan struct {
	App       string       `json:"app"`
	SessionID string       `json:"session_id,omitempty"`
	SourceID  string       `json:"source_id,omitempty"`
	Verdict   string       `json:"verdict"`
	Checks    []Check      `json:"checks"`
	Domains   []DomainPlan `json:"domains"`
	// HealthPath is probed through the ingress, "/" when none is defined.
	HealthPath string `json:"health_path"`
	CheckedAt  string `json:"checked_at"`
}

// DomainRun is one domain's progress and undo data inside a run.
type DomainRun struct {
	Domain string `json:"domain"`
	Method string `json:"method"`
	// Previous is what rollback restores, written before the switch.
	Previous []Record `json:"previous,omitempty"`
	Applied  *Record  `json:"applied,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Zone     string   `json:"zone,omitempty"`
	// Manual is the exact record to set by hand.
	Manual       *Record `json:"manual,omitempty"`
	ProbeStatus  int     `json:"probe_status,omitempty"`
	ProbeDetail  string  `json:"probe_detail,omitempty"`
	Switched     bool    `json:"switched"`
	Restored     bool    `json:"restored"`
	Verified     bool    `json:"verified"`
	VerifyDetail string  `json:"verify_detail,omitempty"`
	Message      string  `json:"message,omitempty"`
}

// Step is one timed entry of a run's timeline.
type Step struct {
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Domain     string    `json:"domain,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	DurationMS int64     `json:"duration_ms"`
}

// Run is one dry run or switch of one app.
type Run struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"session_id"`
	SourceID   string      `json:"source_id"`
	App        string      `json:"app"`
	Mode       string      `json:"mode"`
	State      string      `json:"state"`
	Method     string      `json:"method,omitempty"`
	DNSWrite   bool        `json:"dns_write"`
	WasRouted  bool        `json:"was_routed"`
	AcceptWarn bool        `json:"accept_warnings"`
	Awaiting   string      `json:"awaiting,omitempty"`
	Domains    []DomainRun `json:"domains"`
	Steps      []Step      `json:"steps"`
	Plan       *Plan       `json:"plan,omitempty"`
	Error      string      `json:"error,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	FinishedAt time.Time   `json:"finished_at,omitzero"`
}

// Terminal reports whether the run can no longer change by itself.
func (r Run) Terminal() bool {
	switch r.State {
	case StateReady, StateLive, StateRolledBack, StateFailed:
		return true
	}
	return false
}

// InFlight reports whether a worker is expected to be driving the run.
func (r Run) InFlight() bool {
	switch r.State {
	case StatePlanning, StateStarting, StateVerifying:
		return true
	case StateSwitching:
		return r.Awaiting == ""
	}
	return false
}

// Rollbackable reports whether a rollback can still do something.
func (r Run) Rollbackable() bool {
	if r.Mode != ModeSwitch {
		return false
	}
	switch r.State {
	case StateLive, StateSwitching:
		return true
	case StateFailed:
		for _, d := range r.Domains {
			if (d.Switched || d.Applied != nil || len(d.Previous) > 0) && !d.Restored {
				return true
			}
		}
	}
	return false
}
