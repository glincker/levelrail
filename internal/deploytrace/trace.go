// Package deploytrace records the step-by-step pipeline of a deploy, build or
// not, as rows the step stream, the log stream and the CLI all read.
package deploytrace

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Step names. One constant per concept, shared by emitters and the viewer.
const (
	StepResolveDigest   = "resolve_digest"
	StepRegistryAuth    = "registry_auth"
	StepImagePull       = "image_pull"
	StepImagePullLayer  = "image_pull_layer"
	StepImageInspect    = "image_inspect"
	StepConfigValidate  = "config_validate"
	StepEnsureNetwork   = "ensure_network"
	StepEnsureVolume    = "ensure_volume"
	StepContainerCreate = "container_create"
	StepContainerStart  = "container_start"
	StepDockerEvent     = "docker_event"
	StepReadinessProbe  = "readiness_probe"
	StepReadinessGate   = "readiness_gate"
	StepStabilityGate   = "stability_gate"
	StepCutover         = "cutover"
	StepDrainOld        = "drain_old"
	StepRetainImage     = "retain_image"
	StepComposeParse    = "compose_parse"
	StepComposePlan     = "compose_plan"
	StepComposeService  = "compose_service"
	StepDependsOn       = "depends_on"
	StepSummary         = store.TraceSummaryKey
)

// Sink persists steps. *store.DB satisfies it.
type Sink interface {
	SaveDeployTraceStep(ctx context.Context, s store.DeployTraceStep) error
}

// Resolver maps a reconcile pass (service, image) to its deploy attempt.
type Resolver interface {
	TraceTargetFor(ctx context.Context, serviceName, image string) (attemptID string, sealed bool, err error)
}

// Tracer writes steps for one attempt. A nil *Tracer is valid and records
// nothing, so call sites never nil-check.
type Tracer struct {
	sink    Sink
	attempt string
	service string
	logger  *slog.Logger
	now     func() time.Time
	pending *pendingSteps // set while buffering, see NewBuffered
}

type pendingSteps struct {
	mu    sync.Mutex
	steps []store.DeployTraceStep
}

// New returns a tracer for attemptID, or nil when there is nothing to write to.
func New(sink Sink, attemptID string, logger *slog.Logger) *Tracer {
	if sink == nil || attemptID == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Tracer{sink: sink, attempt: attemptID, logger: logger, now: time.Now}
}

// NewBuffered returns a tracer that holds steps in memory until Flush, for
// work that happens before the deploy attempt row (and so its id) exists.
func NewBuffered() *Tracer {
	return &Tracer{logger: slog.Default(), now: time.Now, pending: &pendingSteps{}}
}

// Clone copies a buffered tracer so one deploy's shared steps can gain
// per-service steps without leaking them to its siblings.
func (t *Tracer) Clone() *Tracer {
	if t == nil || t.pending == nil {
		return t
	}
	t.pending.mu.Lock()
	defer t.pending.mu.Unlock()
	c := *t
	c.pending = &pendingSteps{steps: append([]store.DeployTraceStep(nil), t.pending.steps...)}
	return &c
}

// Has reports whether a buffered step with key exists.
func (t *Tracer) Has(key string) bool {
	if t == nil || t.pending == nil {
		return false
	}
	t.pending.mu.Lock()
	defer t.pending.mu.Unlock()
	for _, s := range t.pending.steps {
		if s.Key == key {
			return true
		}
	}
	return false
}

// Flush writes every buffered step to sink under attemptID, labelled with
// service when the step has none. It can run once per attempt (a compose
// file's shared steps go to each service's attempt).
func (t *Tracer) Flush(ctx context.Context, sink Sink, attemptID, service string) {
	if t == nil || t.pending == nil || sink == nil || attemptID == "" {
		return
	}
	t.pending.mu.Lock()
	steps := append([]store.DeployTraceStep(nil), t.pending.steps...)
	t.pending.mu.Unlock()
	for _, s := range steps {
		s.AttemptID = attemptID
		if s.Service == "" {
			s.Service = service
		}
		if err := sink.SaveDeployTraceStep(context.WithoutCancel(ctx), s); err != nil {
			t.logger.Warn("deploytrace: flush step failed", slog.String("attempt_id", attemptID), slog.String("step", s.Key), slog.String("error", err.Error()))
		}
	}
}

// ForImage resolves the attempt converging (service, image) and returns its
// tracer, or nil when none matches or the attempt's trace is already sealed.
func ForImage(ctx context.Context, sink Sink, res Resolver, service, image string, logger *slog.Logger) *Tracer {
	if sink == nil || res == nil {
		return nil
	}
	id, sealed, err := res.TraceTargetFor(ctx, service, image)
	if err != nil || id == "" || sealed {
		return nil
	}
	t := New(sink, id, logger)
	if t != nil {
		t.service = service
	}
	return t
}

// AttemptID reports which attempt t writes to.
func (t *Tracer) AttemptID() string {
	if t == nil {
		return ""
	}
	return t.attempt
}

// WithService labels steps with a compose service name.
func (t *Tracer) WithService(name string) *Tracer {
	if t == nil {
		return nil
	}
	c := *t
	c.service = name
	return &c
}

// Span is one running step.
type Span struct {
	t       *Tracer
	step    store.DeployTraceStep
	started time.Time
}

// Begin starts a step keyed by its name.
func (t *Tracer) Begin(ctx context.Context, name, detail string) *Span {
	return t.BeginKey(ctx, name, name, detail)
}

// BeginKey starts a step whose key differs from its name (one per layer, probe).
func (t *Tracer) BeginKey(ctx context.Context, name, key, detail string) *Span {
	if t == nil {
		return nil
	}
	now := t.now()
	s := &Span{t: t, started: now, step: store.DeployTraceStep{
		AttemptID: t.attempt, Key: key, Step: name, Service: t.service,
		Status: store.TraceStatusRunning, Detail: detail, StartedAt: now,
	}}
	t.write(ctx, s.step)
	return s
}

// Progress updates a running step's detail without ending it.
func (s *Span) Progress(ctx context.Context, detail string) {
	if s == nil {
		return
	}
	s.step.Detail = detail
	s.t.write(ctx, s.step)
}

// End finishes the step with status, a short reason code and a detail line.
func (s *Span) End(ctx context.Context, status, reason, detail string) {
	if s == nil {
		return
	}
	end := s.t.now()
	s.step.Status, s.step.Reason, s.step.EndedAt = status, reason, end
	if detail != "" {
		s.step.Detail = detail
	}
	s.step.DurationMS = end.Sub(s.started).Milliseconds()
	s.t.write(ctx, s.step)
}

// Done ends the step successfully.
func (s *Span) Done(ctx context.Context, detail string) {
	s.End(ctx, store.TraceStatusDone, "", detail)
}

// Fail ends the step failed, carrying the underlying error text verbatim.
func (s *Span) Fail(ctx context.Context, reason string, err error) {
	s.End(ctx, store.TraceStatusFailed, reason, errText(err))
}

// Note records an instantaneous step.
func (t *Tracer) Note(ctx context.Context, name, status, reason, detail string) {
	t.NoteKey(ctx, name, name, status, reason, detail)
}

// NoteKey is Note with an explicit key.
func (t *Tracer) NoteKey(ctx context.Context, name, key, status, reason, detail string) {
	if t == nil {
		return
	}
	now := t.now()
	t.write(ctx, store.DeployTraceStep{
		AttemptID: t.attempt, Key: key, Step: name, Service: t.service, Status: status,
		Reason: reason, Detail: detail, StartedAt: now, EndedAt: now,
	})
}

func (t *Tracer) write(ctx context.Context, s store.DeployTraceStep) {
	if t.pending != nil {
		t.pending.mu.Lock()
		defer t.pending.mu.Unlock()
		for i := range t.pending.steps {
			if t.pending.steps[i].Key == s.Key {
				t.pending.steps[i] = s
				return
			}
		}
		t.pending.steps = append(t.pending.steps, s)
		return
	}
	// Detach from a cancelled request so the failure that cancelled it is still recorded.
	if err := t.sink.SaveDeployTraceStep(context.WithoutCancel(ctx), s); err != nil {
		t.logger.Warn("deploytrace: save step failed",
			slog.String("attempt_id", t.attempt), slog.String("step", s.Key), slog.String("error", err.Error()))
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// FormatLine renders s as one human log line for the deploy log stream.
func FormatLine(s store.DeployTraceStep) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[trace] %s", s.Step)
	if s.Service != "" {
		fmt.Fprintf(&b, " (%s)", s.Service)
	}
	if k := s.Key; k != "" && k != s.Step && s.Step != StepComposeService {
		fmt.Fprintf(&b, " %s", strings.TrimPrefix(k, s.Step+"."))
	}
	fmt.Fprintf(&b, ": %s", s.Status)
	if s.Reason != "" {
		fmt.Fprintf(&b, " [%s]", s.Reason)
	}
	if s.Detail != "" {
		fmt.Fprintf(&b, " %s", s.Detail)
	}
	if s.Status != store.TraceStatusRunning && s.DurationMS > 0 {
		fmt.Fprintf(&b, " (%s)", (time.Duration(s.DurationMS) * time.Millisecond).Round(time.Millisecond))
	}
	return b.String()
}
