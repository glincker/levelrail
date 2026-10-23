// Package deploytimeline folds the sources that describe one deploy (the
// attempt row, pipeline trace, gate events, probes, hook runs, build log and
// request metrics) into one ordered list of events, the same shape for a live
// stream and for a frozen replay.
package deploytimeline

import "time"

// Kind groups events for rendering. The UI picks an icon per kind.
type Kind string

// Event kinds.
const (
	KindTrigger  Kind = "trigger"
	KindBuild    Kind = "build"
	KindLog      Kind = "log"
	KindPush     Kind = "image_push"
	KindHook     Kind = "hook"
	KindStep     Kind = "step"
	KindProbe    Kind = "readiness"
	KindCutover  Kind = "cutover"
	KindWatch    Kind = "watch"
	KindRollback Kind = "rollback"
	KindOutcome  Kind = "outcome"
	KindTraffic  Kind = "traffic_impact"
)

// Phase is the coarse stage an event belongs to; compare aligns on it.
type Phase string

// Phases, in the order a deploy moves through them.
const (
	PhaseTrigger   Phase = "trigger"
	PhaseBuild     Phase = "build"
	PhasePush      Phase = "push"
	PhaseHooks     Phase = "hooks"
	PhaseDeploy    Phase = "deploy"
	PhaseReadiness Phase = "readiness"
	PhaseCutover   Phase = "cutover"
	PhaseWatch     Phase = "watch"
	PhaseOutcome   Phase = "outcome"
)

// phaseOrder drives both event ordering ties and the phase summary.
var phaseOrder = []Phase{PhaseTrigger, PhaseBuild, PhasePush, PhaseHooks, PhaseDeploy, PhaseReadiness, PhaseCutover, PhaseWatch, PhaseOutcome}

// Status is the outcome of one event.
type Status string

// Event statuses.
const (
	StatusRunning Status = "running"
	StatusOK      Status = "ok"
	StatusFailed  Status = "failed"
	StatusWarning Status = "warning"
	StatusInfo    Status = "info"
)

// Verdict is how the whole deploy ended up.
type Verdict string

// Deploy verdicts.
const (
	VerdictRunning    Verdict = "running"
	VerdictWatching   Verdict = "watching"
	VerdictSucceeded  Verdict = "succeeded"
	VerdictFailed     Verdict = "failed"
	VerdictRolledBack Verdict = "rolled_back"
	VerdictCanceled   Verdict = "canceled"
	VerdictQueued     Verdict = "queued"
)

// TriggerInfo says who or what started the deploy.
type TriggerInfo struct {
	Source        string `json:"source"`
	Actor         string `json:"actor,omitempty"`
	ActorKind     string `json:"actor_kind,omitempty"`
	Branch        string `json:"branch,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	Author        string `json:"author,omitempty"`
	RerunOf       string `json:"rerun_of,omitempty"`
}

// Window is request health over one slice of time.
type Window struct {
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	HasTraffic   bool      `json:"has_traffic"`
	Requests     float64   `json:"requests"`
	RatePerSec   float64   `json:"rate_per_sec"`
	ErrorRate5xx float64   `json:"error_rate_5xx"`
	P95Ms        float64   `json:"p95_ms"`
}

// Impact compares request health just before and just after cutover.
type Impact struct {
	CutoverAt time.Time `json:"cutover_at"`
	WindowMS  int64     `json:"window_ms"`
	Complete  bool      `json:"complete"`
	Before    Window    `json:"before"`
	After     Window    `json:"after"`
}

// Event is one entry on the timeline. ID is stable for the same underlying
// fact, so a stream can send an event again only when its content changed.
type Event struct {
	ID         string       `json:"id"`
	At         time.Time    `json:"at"`
	Kind       Kind         `json:"kind"`
	Phase      Phase        `json:"phase"`
	Status     Status       `json:"status"`
	Code       string       `json:"code"`
	Title      string       `json:"title"`
	Detail     string       `json:"detail,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Service    string       `json:"service,omitempty"`
	Stream     string       `json:"stream,omitempty"`
	DurationMS int64        `json:"duration_ms,omitempty"`
	Trigger    *TriggerInfo `json:"trigger,omitempty"`
	Impact     *Impact      `json:"impact,omitempty"`
}

// PhaseSpan is the time one phase took.
type PhaseSpan struct {
	Phase      Phase     `json:"phase"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMS int64     `json:"duration_ms"`
}

// Timeline is the whole story of one deploy.
type Timeline struct {
	DeployID   string      `json:"deploy_id"`
	App        string      `json:"app"`
	Verdict    Verdict     `json:"verdict"`
	Live       bool        `json:"live"`
	Frozen     bool        `json:"frozen"`
	FrozenAt   *time.Time  `json:"frozen_at,omitempty"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
	Trigger    TriggerInfo `json:"trigger"`
	Events     []Event     `json:"events"`
	Phases     []PhaseSpan `json:"phases"`
	Impact     *Impact     `json:"impact,omitempty"`
	// LogsDropped counts build log lines left out to respect the line cap.
	LogsDropped int `json:"logs_dropped,omitempty"`
}
