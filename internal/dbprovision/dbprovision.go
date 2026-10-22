// Package dbprovision records and summarizes the steps of creating a database,
// so the dashboard, CLI and API all show the same truth.
package dbprovision

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Step names, in the order they run.
const (
	StepAccepted        = "accepted"
	StepDockerCheck     = "docker_check"
	StepPullImage       = "pull_image"
	StepCreateVolume    = "create_volume"
	StepCredentials     = "credentials_and_tls"
	StepCreateContainer = "create_container"
	StepStart           = "start"
	StepWaitReady       = "wait_ready"
	StepApplySettings   = "apply_settings"
	StepReady           = "ready"
)

// Steps is the canonical order.
var Steps = []string{
	StepAccepted, StepDockerCheck, StepPullImage, StepCreateVolume, StepCredentials,
	StepCreateContainer, StepStart, StepWaitReady, StepApplySettings, StepReady,
}

// Step statuses.
const (
	StatusPending = "pending"
	StatusStarted = "started"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Attempt statuses.
const (
	AttemptInProgress = "in_progress"
	AttemptFailed     = "failed"
	AttemptReady      = "ready"
)

// Fix ids a client can act on.
const (
	FixRecheckDocker = "recheck_docker"
	FixRetry         = "retry"
	FixRemoveVolume  = "remove_leftover_volume"
	FixFreeDisk      = "free_disk"
	FixReuseVolume   = "reuse_existing_volume"
	FixChooseName    = "choose_another_name"
)

// ReasonLeftoverVolume is the Ready-condition reason of the create_volume stop.
const ReasonLeftoverVolume = "LeftoverVolume"

// LeftoverVolumeError is the create_volume stop for a data volume a deleted
// database of the same name left behind.
type LeftoverVolumeError struct {
	Name      string
	SizeBytes int64
}

func (e *LeftoverVolumeError) Error() string {
	return fmt.Sprintf("leftover volume %s (%d bytes) from a deleted database of the same name", e.Name, e.SizeBytes)
}

var leftoverRe = regexp.MustCompile(`leftover volume (\S+) \((-?\d+) bytes\)`)

// ParseLeftover extracts the volume name and size from a LeftoverVolumeError text.
func ParseLeftover(text string) (VolumeInfo, bool) {
	m := leftoverRe.FindStringSubmatch(text)
	if m == nil {
		return VolumeInfo{}, false
	}
	size, _ := strconv.ParseInt(m[2], 10, 64)
	return VolumeInfo{Name: m[1], SizeBytes: size}, true
}

// VolumeInfo names the leftover volume a failure is about.
type VolumeInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// Environment variables.
const (
	EnvKeepAttempts  = "APP_DB_PROVISION_KEEP_ATTEMPTS"
	EnvEventInterval = "APP_DB_PROVISION_EVENT_INTERVAL"
	defaultKeep      = 5
	defaultInterval  = 500 * time.Millisecond
)

// Store is what the recorder needs; *store.DB satisfies it.
type Store interface {
	AppendDatabaseProvisionEvent(ctx context.Context, e store.DatabaseProvisionEvent) error
	BeginDatabaseProvisionAttempt(ctx context.Context, name string, keep int) (string, error)
	LatestDatabaseProvisionAttempt(ctx context.Context, name string) (string, []store.DatabaseProvisionEvent, error)
}

// Recorder writes provisioning events.
type Recorder struct {
	store    Store
	keep     int
	interval time.Duration
	now      func() time.Time
	mu       sync.Mutex
	last     map[string]string
}

// NewRecorder reads the attempt and throttle settings from the environment.
func NewRecorder(s Store) *Recorder {
	r := &Recorder{store: s, keep: defaultKeep, interval: defaultInterval, now: time.Now, last: map[string]string{}}
	if n, err := strconv.Atoi(os.Getenv(EnvKeepAttempts)); err == nil && n > 0 {
		r.keep = n
	}
	if d, err := time.ParseDuration(os.Getenv(EnvEventInterval)); err == nil && d >= 0 {
		r.interval = d
	}
	return r
}

// SetClock replaces the time source, for tests.
func (r *Recorder) SetClock(now func() time.Time) { r.now = now }

// Begin starts a new attempt and records the accepted step.
func (r *Recorder) Begin(ctx context.Context, name string) (*Tracker, error) {
	id, err := r.store.BeginDatabaseProvisionAttempt(ctx, name, r.keep)
	if err != nil {
		return nil, fmt.Errorf("dbprovision: begin attempt for %q: %w", name, err)
	}
	t := &Tracker{r: r, name: name, id: id, ctx: ctx}
	t.Done(StepAccepted, "request accepted")
	return t, nil
}

// Open returns a tracker for name's newest attempt unless it already reached
// ready, or nil when the database never had an attempt.
func (r *Recorder) Open(ctx context.Context, name string) *Tracker {
	id, events, err := r.store.LatestDatabaseProvisionAttempt(ctx, name)
	if err != nil || id == "" {
		return nil
	}
	if View(id, events).Status == AttemptReady {
		return nil
	}
	return &Tracker{r: r, name: name, id: id, ctx: ctx}
}

// Tracker writes one attempt's events. A nil Tracker ignores every call.
type Tracker struct {
	r        *Recorder
	name, id string
	ctx      context.Context
	lastProg time.Time
	pulling  bool
	pullDone bool
}

func (t *Tracker) emit(step, status, detail string, cur, total int64) {
	if t == nil {
		return
	}
	key := t.name + "/" + step
	sig := t.id + "|" + status + "|" + detail
	t.r.mu.Lock()
	dup := t.r.last[key] == sig && total == 0
	t.r.last[key] = sig
	t.r.mu.Unlock()
	if dup {
		return
	}
	_ = t.r.store.AppendDatabaseProvisionEvent(t.ctx, store.DatabaseProvisionEvent{
		Database: t.name, AttemptID: t.id, Step: step, Status: status, Detail: detail,
		Current: cur, Total: total, At: t.r.now(),
	})
}

// Start marks step as running.
func (t *Tracker) Start(step, detail string) { t.emit(step, StatusStarted, detail, 0, 0) }

// Done marks step as finished.
func (t *Tracker) Done(step, detail string) { t.emit(step, StatusDone, detail, 0, 0) }

// Skip marks step as not needed.
func (t *Tracker) Skip(step, detail string) { t.emit(step, StatusSkipped, detail, 0, 0) }

// Progress records progress for step, no more often than the throttle.
func (t *Tracker) Progress(step, detail string, cur, total int64) {
	if t == nil {
		return
	}
	now := t.r.now()
	if !t.lastProg.IsZero() && now.Sub(t.lastProg) < t.r.interval {
		return
	}
	t.lastProg = now
	t.emit(step, StatusStarted, detail, cur, max(total, 1))
}

// Pull converts a Docker pull report into pull_image events.
func (t *Tracker) Pull(ev docker.PullEvent) {
	if t == nil {
		return
	}
	switch ev.Phase {
	case docker.PullPhaseStart:
		t.pulling = true
		t.Start(StepPullImage, "pulling "+ev.Ref)
	case docker.PullPhaseLayer:
		t.Progress(StepPullImage, fmt.Sprintf("layer %s: %s", ev.LayerID, ev.LayerState), 0, 100)
	case docker.PullPhaseProgress:
		d := fmt.Sprintf("%d of %d layers", ev.LayersDone, ev.Layers)
		t.lastProg = time.Time{}
		t.Progress(StepPullImage, d, int64(ev.Percent), 100)
	case docker.PullPhaseDone:
		t.pullDone = true
		if ev.Status == docker.PullStatusCached {
			t.Done(StepPullImage, "image already present")
			return
		}
		t.Done(StepPullImage, "image pulled")
	}
}

// PullFinished reports whether the pull step completed.
func (t *Tracker) PullFinished() bool { return t != nil && (t.pullDone || !t.pulling) }

// Fail marks step failed and every later step skipped.
func (t *Tracker) Fail(step string, err error) {
	if t == nil {
		return
	}
	msg := "failed"
	if err != nil {
		msg = err.Error()
	}
	t.emit(step, StatusFailed, msg, 0, 0)
	after := false
	for _, s := range Steps {
		if after {
			t.emit(s, StatusSkipped, "", 0, 0)
		}
		if s == step {
			after = true
		}
	}
}

// Finish marks every remaining step done and the attempt ready.
func (t *Tracker) Finish() {
	if t == nil {
		return
	}
	id, events, err := t.r.store.LatestDatabaseProvisionAttempt(t.ctx, t.name)
	if err != nil || id != t.id {
		return
	}
	cur := View(id, events)
	for _, s := range cur.Steps {
		if s.Status != StatusDone && s.Step != StepReady {
			t.emit(s.Step, StatusDone, s.Detail, 0, 0)
		}
	}
	t.emit(StepReady, StatusDone, "ready for connections", 0, 0)
}

// FailStepForReason maps a reconciler Ready-condition reason to the step it
// belongs to; "" means the reason is not a provisioning failure.
func FailStepForReason(reason string, pullFinished bool, errText string) string {
	switch reason {
	case "CredentialsNotConfigured", "TLSProvisionFailed", "UnauthenticatedExposureRefused", "BindAddressInvalid":
		return StepCredentials
	case "VolumeFailed":
		if ClassifyFix(StepCreateVolume, errText) != nil && ClassifyFix(StepCreateVolume, errText).ID == FixRecheckDocker {
			return StepDockerCheck
		}
		return StepCreateVolume
	case "InspectFailed":
		return StepDockerCheck
	case ReasonLeftoverVolume:
		return StepCreateVolume
	case "NotAcceptingConnections":
		return StepWaitReady
	case "CreateFailed", "ReplaceFailed":
		if !pullFinished || strings.Contains(errText, "pull image") {
			return StepPullImage
		}
		return StepCreateContainer
	case "StartFailed", "StartFailedAfterCreate":
		return StepStart
	case "VanishedAfterStart":
		return StepWaitReady
	case "UnknownEngine", "StoreError":
		return StepAccepted
	}
	return ""
}

// Fix is a typed way to recover from a failed step.
type Fix struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Confirm bool   `json:"confirm,omitempty"`
}

// ClassifyFix suggests a fix for a failure, or nil when none is known.
func ClassifyFix(step, errText string) *Fix {
	e := strings.ToLower(errText)
	switch {
	case strings.Contains(e, "leftover volume") && strings.Contains(e, "bytes)"):
		return &Fix{ID: FixRemoveVolume, Label: "Remove leftover volume and retry", Confirm: true}
	case strings.Contains(e, "docker-guard.sock") || strings.Contains(e, "not responding") ||
		strings.Contains(e, "cannot connect to the docker daemon"):
		return &Fix{ID: FixRecheckDocker, Label: "Re-check Docker"}
	case strings.Contains(e, "no space left"):
		return &Fix{ID: FixFreeDisk, Label: "Free up disk space"}
	case step == StepPullImage || strings.Contains(e, "not found") && strings.Contains(e, "image") ||
		strings.Contains(e, "no such host") || strings.Contains(e, "registry"):
		return &Fix{ID: FixRetry, Label: "Retry"}
	case strings.Contains(e, "volume") && (strings.Contains(e, "in use") || strings.Contains(e, "exists") || strings.Contains(e, "leftover")):
		return &Fix{ID: FixRemoveVolume, Label: "Remove leftover volume and retry", Confirm: true}
	case strings.Contains(e, "port is already allocated") || strings.Contains(e, "address already in use") || strings.Contains(e, "name is already in use") || strings.Contains(e, "conflict"):
		return &Fix{ID: FixRetry, Label: "Retry"}
	}
	return nil
}

// StepView is one step of an attempt as clients render it.
type StepView struct {
	Step    string     `json:"step"`
	Status  string     `json:"status"`
	Detail  string     `json:"detail,omitempty"`
	Current int64      `json:"current,omitempty"`
	Total   int64      `json:"total,omitempty"`
	At      *time.Time `json:"at,omitempty"`
}

// EventView is one raw event for the Details log.
type EventView struct {
	Step   string    `json:"step"`
	Status string    `json:"status"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

// Failure describes why an attempt stopped.
type Failure struct {
	Step   string `json:"step"`
	Reason string `json:"reason"`
	Fix    *Fix   `json:"fix,omitempty"`
	// Fixes lists every choice when the stop needs the owner to decide.
	Fixes  []Fix       `json:"fixes,omitempty"`
	Volume *VolumeInfo `json:"volume,omitempty"`
}

// Attempt is the latest provisioning attempt of a database.
type Attempt struct {
	ID          string      `json:"attempt_id"`
	Status      string      `json:"status"`
	CurrentStep string      `json:"current_step,omitempty"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	Steps       []StepView  `json:"steps"`
	Events      []EventView `json:"events"`
	Failure     *Failure    `json:"failure,omitempty"`
}

// View folds an attempt's events into per-step state.
func View(id string, events []store.DatabaseProvisionEvent) Attempt {
	a := Attempt{ID: id, Status: AttemptInProgress, Steps: make([]StepView, 0, len(Steps)), Events: []EventView{}}
	idx := map[string]int{}
	for i, s := range Steps {
		a.Steps = append(a.Steps, StepView{Step: s, Status: StatusPending})
		idx[s] = i
	}
	for _, e := range events {
		i, ok := idx[e.Step]
		if !ok {
			continue
		}
		at := e.At
		a.Steps[i] = StepView{Step: e.Step, Status: e.Status, Detail: e.Detail, Current: e.Current, Total: e.Total, At: &at}
		a.Events = append(a.Events, EventView{Step: e.Step, Status: e.Status, Detail: e.Detail, At: e.At})
		if a.StartedAt == nil {
			a.StartedAt = &at
		}
		if e.Step == StepReady && e.Status == StatusDone {
			a.FinishedAt = &at
		}
	}
	for _, sv := range a.Steps {
		if sv.Status == StatusFailed {
			a.Failure = &Failure{Step: sv.Step, Reason: sv.Detail, Fix: ClassifyFix(sv.Step, sv.Detail)}
			if v, ok := ParseLeftover(sv.Detail); ok {
				a.Failure.Volume = &v
				a.Failure.Fixes = []Fix{
					{ID: FixReuseVolume, Label: "Reuse the existing volume"},
					{ID: FixRemoveVolume, Label: "Remove leftover volume and retry", Confirm: true},
					{ID: FixChooseName, Label: "Choose another name"},
				}
			}
			break
		}
	}
	switch {
	case a.Steps[len(Steps)-1].Status == StatusDone:
		a.Status = AttemptReady
	case a.Failure != nil:
		a.Status = AttemptFailed
	}
	for _, s := range a.Steps {
		if s.Status == StatusStarted || (a.CurrentStep == "" && s.Status == StatusPending && a.Status == AttemptInProgress) {
			a.CurrentStep = s.Step
			if s.Status == StatusStarted {
				break
			}
		}
	}
	if a.Failure != nil {
		a.CurrentStep = a.Failure.Step
	}
	return a
}
