package deploytimeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
)

// LogLine is one line of build output.
type LogLine struct {
	At      time.Time
	Stream  string
	Message string
}

// Inputs are the raw sources for one deploy. Any of them may be empty: a
// deploy that never built has no log, one without a safety gate has no gate
// events.
type Inputs struct {
	Attempt store.DeployAttempt
	Trace   []store.DeployTraceStep
	Gate    []store.DeployGateEvent
	Probes  []store.ProbeAttempt
	Hooks   []store.HookRun
	Logs    []LogLine
	// Actor and ActorKind come from the audit trail when it matched.
	Actor     string
	ActorKind string
	Impact    *Impact
	// WatchOpen is true while the post-deploy watch can still trip.
	WatchOpen bool
	Now       time.Time
	Config    Config
}

const (
	statusDone    = store.TraceStatusDone
	statusFailed  = store.TraceStatusFailed
	statusRunning = store.TraceStatusRunning
	statusWarning = store.TraceStatusWarning
)

func traceStatus(s string) Status {
	switch s {
	case statusDone:
		return StatusOK
	case statusFailed:
		return StatusFailed
	case statusRunning:
		return StatusRunning
	case statusWarning:
		return StatusWarning
	}
	return StatusInfo
}

func gateStatus(s string) Status {
	switch s {
	case deploysafety.StatusPassed:
		return StatusOK
	case deploysafety.StatusFailed:
		return StatusFailed
	case deploysafety.StatusStarted:
		return StatusRunning
	}
	return StatusInfo
}

func gatePhase(p string) (Kind, Phase) {
	switch p {
	case deploysafety.PhaseReadiness, deploysafety.PhaseStability:
		return KindProbe, PhaseReadiness
	case deploysafety.PhaseCutover, deploysafety.PhaseDrain:
		return KindCutover, PhaseCutover
	case deploysafety.PhaseWatch:
		return KindWatch, PhaseWatch
	case deploysafety.PhaseRollback:
		return KindRollback, PhaseWatch
	}
	return KindStep, PhaseDeploy
}

// watchCode names a post-deploy watch signal from the gate reason.
func watchCode(e store.DeployGateEvent) string {
	switch e.Reason {
	case deploysafety.ReasonWatchCrashloop:
		return "watch.crashloop"
	case deploysafety.ReasonWatchOOMKilled:
		return "watch.oom"
	case deploysafety.ReasonWatchExited:
		return "watch.exited"
	case deploysafety.ReasonWatchErrorRate:
		return "watch.error_rate"
	case deploysafety.ReasonWatchLatency:
		return "watch.latency"
	case deploysafety.ReasonWatchStopped:
		return "watch.stopped"
	}
	switch e.Status {
	case deploysafety.StatusPassed:
		return "watch.cleared"
	case deploysafety.StatusStarted:
		return "watch.started"
	case deploysafety.StatusFailed:
		return "watch.tripped"
	}
	return "watch.notice"
}

func gateCode(e store.DeployGateEvent) string {
	switch e.Phase {
	case deploysafety.PhaseWatch:
		return watchCode(e)
	case deploysafety.PhaseRollback:
		if e.Reason == deploysafety.ReasonNoRollbackTarget {
			return "rollback.no_target"
		}
		return "rollback.auto"
	}
	return e.Phase + "." + e.Status
}

func gateTitle(e store.DeployGateEvent) string {
	code := gateCode(e)
	switch code {
	case "watch.crashloop":
		return "Crashloop after deploy"
	case "watch.oom":
		return "Container killed for memory after deploy"
	case "watch.exited":
		return "Container exited after deploy"
	case "watch.error_rate":
		return "5xx rate rose after deploy"
	case "watch.latency":
		return "Latency regressed after deploy"
	case "watch.stopped":
		return "Watch stopped by an operator"
	case "watch.cleared":
		return "Post-deploy watch cleared"
	case "rollback.auto":
		return "Automatic rollback"
	case "rollback.no_target":
		return "No rollback target available"
	}
	return strings.TrimSpace(humanize(e.Phase) + " " + e.Status)
}

// coarse build steps the build handler writes before the reconciler starts.
func coarseStep(step string) (Kind, Phase, string, bool) {
	switch step {
	case "detecting":
		return KindBuild, PhaseBuild, "Detecting how to build", true
	case "building":
		return KindBuild, PhaseBuild, "Build", true
	case "pushing":
		return KindPush, PhasePush, "Image push", true
	case "deploying":
		return KindStep, PhaseDeploy, "Handing off to the reconciler", true
	case "failed":
		return KindBuild, PhaseBuild, "Build failed", true
	}
	return "", "", "", false
}

// traceGatePhase maps the trace steps a gate event supersedes to the gate
// phase that carries the same fact with its verdict.
var traceGatePhase = map[string]string{
	deploytrace.StepReadinessGate: deploysafety.PhaseReadiness,
	deploytrace.StepStabilityGate: deploysafety.PhaseStability,
	deploytrace.StepCutover:       deploysafety.PhaseCutover,
	deploytrace.StepDrainOld:      deploysafety.PhaseDrain,
}

func traceEvent(s store.DeployTraceStep) (Event, bool) {
	if s.Step == deploytrace.StepSummary || s.Step == deploytrace.StepImagePullLayer {
		return Event{}, false
	}
	if s.Step == deploytrace.StepDockerEvent && s.Status == statusDone {
		return Event{}, false
	}
	dur := s.DurationMS
	if dur == 0 && !s.EndedAt.IsZero() {
		dur = s.EndedAt.Sub(s.StartedAt).Milliseconds()
	}
	e := Event{
		ID: "trace:" + s.Key, At: s.StartedAt, Status: traceStatus(s.Status), Reason: s.Reason, Detail: s.Detail,
		Service: s.Service, DurationMS: dur, Kind: KindStep, Phase: PhaseDeploy, Code: "step." + s.Step,
		Title: humanize(s.Step),
	}
	if kind, phase, title, ok := coarseStep(s.Step); ok {
		e.Kind, e.Phase, e.Title, e.Code = kind, phase, title, kindCode(kind, s.Step)
		return e, true
	}
	switch s.Step {
	case deploytrace.StepReadinessProbe, deploytrace.StepReadinessGate, deploytrace.StepStabilityGate:
		e.Kind, e.Phase = KindProbe, PhaseReadiness
	case deploytrace.StepCutover, deploytrace.StepDrainOld:
		e.Kind, e.Phase = KindCutover, PhaseCutover
	}
	return e, true
}

func kindCode(k Kind, step string) string {
	switch k {
	case KindPush:
		return "push"
	case KindBuild:
		if step == "failed" {
			return "build.failed"
		}
		return "build." + step
	}
	return "step." + step
}

func humanize(step string) string {
	s := strings.ReplaceAll(step, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sha7(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func triggerEvent(in Inputs) (Event, TriggerInfo) {
	a := in.Attempt
	info := TriggerInfo{
		Source: a.Source, Actor: in.Actor, ActorKind: in.ActorKind, Branch: a.Branch, CommitSHA: a.CommitSHA,
		CommitMessage: firstLine(a.CommitMessage), Author: a.Author, RerunOf: a.RerunOf,
	}
	e := Event{ID: "trigger", At: a.StartedAt, Kind: KindTrigger, Phase: PhaseTrigger, Status: StatusInfo, Trigger: &info}
	switch a.Source {
	case store.DeployAttemptSourceWebhook:
		e.Code, e.Title = "trigger.webhook", "Webhook received"
	case store.DeployAttemptSourceGitPush:
		e.Code, e.Title = "trigger.git_push", "Git push received"
	case store.DeployAttemptSourceManual:
		e.Code, e.Title = "trigger.manual", "Deploy started manually"
	case store.DeployAttemptSourceImage:
		e.Code, e.Title = "trigger.image", "Image deploy requested"
	case store.DeployAttemptSourceCompose:
		e.Code, e.Title = "trigger.compose", "Compose deploy requested"
	case store.DeployAttemptSourcePromote:
		e.Code, e.Title = "trigger.promote", "Promotion requested"
	case store.DeployAttemptSourceRestart:
		e.Code, e.Title = "trigger.restart", "Restart requested"
	case store.DeployAttemptSourceAutoRollback:
		e.Code, e.Title = "trigger.auto_rollback", "Automatic rollback started"
	default:
		e.Code, e.Title = "trigger.unknown", "Deploy started"
	}
	var parts []string
	if a.Branch != "" || a.CommitSHA != "" {
		parts = append(parts, strings.TrimSpace(a.Branch+" "+sha7(a.CommitSHA)))
	}
	if info.CommitMessage != "" {
		parts = append(parts, info.CommitMessage)
	}
	if actor := firstNonEmpty(in.Actor, a.Author); actor != "" {
		parts = append(parts, "by "+actor)
	}
	e.Detail = strings.Join(parts, " | ")
	return e, info
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func probeEvent(p store.ProbeAttempt) Event {
	status := StatusOK
	detail := p.Target
	switch {
	case p.StatusCode != 0:
		detail += fmt.Sprintf(" HTTP %d", p.StatusCode)
	case p.ExitCode != 0:
		detail += fmt.Sprintf(" exit %d", p.ExitCode)
	}
	detail += fmt.Sprintf(" %dms", p.LatencyMS)
	if !p.Success {
		status = StatusFailed
		if p.Error != "" {
			detail += ": " + p.Error
		}
	}
	return Event{
		ID: fmt.Sprintf("probe:%d", p.ID), At: p.ProbedAt, Kind: KindProbe, Phase: PhaseReadiness, Status: status,
		Code: "readiness.probe", Title: "Readiness probe", Detail: detail, DurationMS: p.LatencyMS,
	}
}

func hookEvent(h store.HookRun) Event {
	status, title := StatusOK, "Hook passed"
	if !h.Success {
		status, title = StatusFailed, "Hook failed"
	}
	label := strings.ReplaceAll(h.HookType, "_", "-")
	detail := h.Command
	if !h.Success {
		detail = fmt.Sprintf("%s (exit %d)", h.Command, h.ExitCode)
	}
	return Event{
		ID: "hook:" + h.HookType, At: h.RanAt, Kind: KindHook, Phase: PhaseHooks, Status: status,
		Code: "hook." + h.HookType, Title: strings.TrimSpace(label + " " + strings.ToLower(title)), Detail: detail,
	}
}

// Assemble builds the ordered timeline. It is pure: the same inputs always
// give the same events, so a stream can diff successive calls.
func Assemble(in Inputs) Timeline {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	a := in.Attempt
	trig, info := triggerEvent(in)
	events := []Event{trig}

	if a.QueuedAt != nil {
		events = append(events, Event{
			ID: "queued", At: *a.QueuedAt, Kind: KindTrigger, Phase: PhaseTrigger, Status: StatusInfo,
			Code: "trigger.queued", Title: "Queued behind another deploy", Reason: a.Reason,
		})
	}

	gatePhases := map[string]bool{}
	for _, g := range in.Gate {
		gatePhases[g.Phase] = true
	}
	for _, s := range in.Trace {
		if gp, ok := traceGatePhase[s.Step]; ok && gatePhases[gp] {
			continue
		}
		if e, ok := traceEvent(s); ok {
			events = append(events, e)
		}
	}
	for _, g := range in.Gate {
		if g.Phase == deploysafety.PhasePin {
			continue
		}
		kind, phase := gatePhase(g.Phase)
		events = append(events, Event{
			ID: fmt.Sprintf("gate:%d", g.ID), At: g.At, Kind: kind, Phase: phase, Status: gateStatus(g.Status),
			Code: gateCode(g), Title: gateTitle(g), Detail: g.Message, Reason: g.Reason,
		})
	}
	for _, p := range in.Probes {
		events = append(events, probeEvent(p))
	}
	end := in.Now
	if a.FinishedAt != nil {
		end = *a.FinishedAt
	}
	hookUntil := end.Add(in.Config.ImpactWindow)
	for _, h := range in.Hooks {
		if !h.RanAt.Before(a.StartedAt) && !h.RanAt.After(hookUntil) {
			events = append(events, hookEvent(h))
		}
	}

	logEvents, dropped := logEvents(in.Logs, in.Config.MaxLogLines)
	events = append(events, logEvents...)

	verdict := verdictOf(in)
	if a.FinishedAt != nil {
		events = append(events, outcomeEvent(a, verdict, outcomeAt(in, verdict)))
	}
	if in.Impact != nil {
		events = append(events, impactEvent(*in.Impact, in.Now))
	}

	// Pipeline steps can be recorded before the attempt row exists.
	for _, e := range events[1:] {
		if e.At.Before(events[0].At) {
			events[0].At = e.At
		}
	}
	sortEvents(events)
	tl := Timeline{
		DeployID: a.ID, App: a.ServiceName, Verdict: verdict, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
		Trigger: info, Events: events, Impact: in.Impact, LogsDropped: dropped,
		Live: a.FinishedAt == nil || in.WatchOpen,
	}
	tl.Phases = phaseSpans(events)
	return tl
}

func logEvents(lines []LogLine, limit int) ([]Event, int) {
	dropped := 0
	if limit > 0 && len(lines) > limit {
		dropped = len(lines) - limit
		lines = lines[len(lines)-limit:]
	}
	out := make([]Event, 0, len(lines))
	for i, l := range lines {
		status := StatusInfo
		if l.Stream == "stderr" {
			status = StatusWarning
		}
		out = append(out, Event{
			ID: fmt.Sprintf("log:%d", dropped+i), At: l.At, Kind: KindLog, Phase: PhaseBuild, Status: status,
			Code: "log.line", Title: "Build output", Detail: l.Message, Stream: l.Stream,
		})
	}
	return out, dropped
}

func verdictOf(in Inputs) Verdict {
	a := in.Attempt
	rolledBack := false
	for _, g := range in.Gate {
		if g.Phase == deploysafety.PhaseRollback && g.Status == deploysafety.StatusPassed {
			rolledBack = true
		}
	}
	switch {
	case a.Status == store.DeployAttemptStatusQueued || a.Status == store.DeployAttemptStatusHeld:
		return VerdictQueued
	case a.Status == store.DeployAttemptStatusCanceled || a.Status == store.DeployAttemptStatusSuperseded:
		return VerdictCanceled
	case rolledBack:
		return VerdictRolledBack
	case a.Status == store.DeployAttemptStatusFailed || summaryFailed(in.Trace):
		return VerdictFailed
	case a.FinishedAt == nil:
		return VerdictRunning
	case in.WatchOpen:
		return VerdictWatching
	case a.Status == store.DeployAttemptStatusSucceeded:
		return VerdictSucceeded
	}
	return VerdictRunning
}

func summaryFailed(trace []store.DeployTraceStep) bool {
	for _, s := range trace {
		if s.Step == deploytrace.StepSummary && s.Status == statusFailed {
			return true
		}
	}
	return false
}

// outcomeAt is when the story ends. An attempt can finish at hand-off while
// the reconciler is still converging, so the outcome waits for the closing
// summary, cutover and any rollback.
func outcomeAt(in Inputs, v Verdict) time.Time {
	at := *in.Attempt.FinishedAt
	later := func(t time.Time) {
		if t.After(at) {
			at = t
		}
	}
	for _, s := range in.Trace {
		if s.Step == deploytrace.StepSummary && s.Status != statusRunning {
			later(s.EndedAt)
		}
	}
	if cut, ok := CutoverAt(in.Gate, in.Trace); ok {
		later(cut)
	}
	if v == VerdictRolledBack {
		for _, g := range in.Gate {
			if g.Phase == deploysafety.PhaseRollback {
				later(g.At)
			}
		}
	}
	return at
}

func outcomeEvent(a store.DeployAttempt, v Verdict, at time.Time) Event {
	e := Event{ID: "outcome", At: at, Kind: KindOutcome, Phase: PhaseOutcome}
	switch v {
	case VerdictFailed:
		e.Status, e.Code, e.Title, e.Detail = StatusFailed, "outcome.failed", "Deploy failed", a.Error
	case VerdictRolledBack:
		e.Status, e.Code, e.Title = StatusWarning, "outcome.rolled_back", "Deploy rolled back"
	case VerdictCanceled:
		e.Status, e.Code, e.Title, e.Detail = StatusInfo, "outcome.canceled", "Deploy canceled", a.Reason
	default:
		e.Status, e.Code, e.Title = StatusOK, "outcome.succeeded", "Deploy finished"
	}
	return e
}

func impactEvent(im Impact, now time.Time) Event {
	at := im.CutoverAt.Add(time.Duration(im.WindowMS) * time.Millisecond)
	if !im.Complete {
		at = now
	}
	copyOf := im
	return Event{
		ID: "traffic:impact", At: at, Kind: KindTraffic, Phase: PhaseWatch, Status: StatusInfo,
		Code: "traffic.impact", Title: "Traffic impact around cutover", Impact: &copyOf,
	}
}

func phaseRank(p Phase) int {
	for i, q := range phaseOrder {
		if p == q {
			return i
		}
	}
	return len(phaseOrder)
}

func sortEvents(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if ra, rb := phaseRank(a.Phase), phaseRank(b.Phase); ra != rb {
			return ra < rb
		}
		return a.ID < b.ID
	})
}

// phaseSpans measures each phase from its first to its last non-log event,
// counting a step's own duration forwards from when it began.
func phaseSpans(events []Event) []PhaseSpan {
	type bounds struct{ start, end time.Time }
	byPhase := map[Phase]*bounds{}
	for _, e := range events {
		if e.Kind == KindLog {
			continue
		}
		end := e.At.Add(time.Duration(e.DurationMS) * time.Millisecond)
		b, ok := byPhase[e.Phase]
		if !ok {
			byPhase[e.Phase] = &bounds{start: e.At, end: end}
			continue
		}
		if e.At.Before(b.start) {
			b.start = e.At
		}
		if end.After(b.end) {
			b.end = end
		}
	}
	out := make([]PhaseSpan, 0, len(byPhase))
	for _, p := range phaseOrder {
		if b, ok := byPhase[p]; ok {
			out = append(out, PhaseSpan{Phase: p, StartedAt: b.start, EndedAt: b.end, DurationMS: b.end.Sub(b.start).Milliseconds()})
		}
	}
	return out
}

// CutoverAt returns when traffic moved to the new release, from the gate
// timeline first and the pipeline trace otherwise.
func CutoverAt(gate []store.DeployGateEvent, trace []store.DeployTraceStep) (time.Time, bool) {
	for _, g := range gate {
		if g.Phase == deploysafety.PhaseCutover && g.Status == deploysafety.StatusPassed {
			return g.At, true
		}
	}
	for _, s := range trace {
		if s.Step == deploytrace.StepCutover && s.Status == statusDone {
			if !s.EndedAt.IsZero() {
				return s.EndedAt, true
			}
			return s.StartedAt, true
		}
	}
	return time.Time{}, false
}

// Settled reports whether nothing more will be added to the timeline, which
// is when it is safe to freeze it for replay.
func Settled(in Inputs) bool {
	a := in.Attempt
	if a.FinishedAt == nil || in.WatchOpen {
		return false
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if cut, ok := CutoverAt(in.Gate, in.Trace); ok && now.Before(cut.Add(in.Config.ImpactWindow)) {
		return false
	}
	for _, s := range in.Trace {
		if s.Step == deploytrace.StepSummary && s.Status != statusRunning {
			return true
		}
	}
	switch a.Status {
	case store.DeployAttemptStatusFailed, store.DeployAttemptStatusCanceled, store.DeployAttemptStatusSuperseded:
		return true
	}
	return now.After(a.FinishedAt.Add(in.Config.SettleIdle))
}

// Signature changes whenever an event's visible content does. A log line's
// time is ignored: live lines carry a placeholder until the persisted log lands.
func (e Event) Signature() string {
	sig := fmt.Sprintf("%s|%s|%s|%s|%d", e.Status, e.Reason, e.Detail, e.Title, e.DurationMS)
	if e.Kind != KindLog {
		sig += fmt.Sprintf("|%d", e.At.UnixNano())
	}
	if e.Impact != nil {
		sig += fmt.Sprintf("|%t|%.4f|%.4f|%.2f|%.2f", e.Impact.Complete, e.Impact.After.RatePerSec, e.Impact.After.ErrorRate5xx, e.Impact.After.P95Ms, e.Impact.Before.P95Ms)
	}
	return sig
}
