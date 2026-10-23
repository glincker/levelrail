package deploytimeline

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func at(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

func finished(s int) *time.Time { v := at(s); return &v }

func baseInputs() Inputs {
	return Inputs{
		Attempt: store.DeployAttempt{
			ID: "dep_1", ServiceName: "web", Source: store.DeployAttemptSourceGitPush, Branch: "main",
			CommitSHA: "abcdef1234567", CommitMessage: "fix login\n\nbody", Author: "ana",
			Status: store.DeployAttemptStatusSucceeded, StartedAt: at(0), FinishedAt: finished(60),
		},
		Trace: []store.DeployTraceStep{
			{Key: "building", Step: "building", Status: store.TraceStatusDone, StartedAt: at(1), EndedAt: at(30), DurationMS: 29000},
			{Key: "pushing", Step: "pushing", Status: store.TraceStatusDone, StartedAt: at(30), EndedAt: at(40), DurationMS: 10000},
			{Key: "readiness_probe", Step: "readiness_probe", Status: store.TraceStatusDone, StartedAt: at(41), EndedAt: at(45)},
			{Key: "cutover", Step: "cutover", Status: store.TraceStatusDone, StartedAt: at(46), EndedAt: at(47)},
			{Key: "image_pull_layer:1", Step: "image_pull_layer", Status: store.TraceStatusDone, StartedAt: at(2), EndedAt: at(3)},
			{Key: "deploy_summary", Step: "deploy_summary", Status: store.TraceStatusDone, StartedAt: at(58), EndedAt: at(59)},
		},
		Logs:   []LogLine{{At: at(5), Stream: "stdout", Message: "step 1"}, {At: at(6), Stream: "stderr", Message: "warn"}},
		Now:    at(120),
		Config: Config{MaxLogLines: 10, ImpactWindow: time.Minute, SettleIdle: time.Minute},
	}
}

func codes(tl Timeline) []string {
	out := make([]string, 0, len(tl.Events))
	for _, e := range tl.Events {
		out = append(out, e.Code)
	}
	return out
}

func TestAssembleOrdersTheStory(t *testing.T) {
	tl := Assemble(baseInputs())
	want := []string{"trigger.git_push", "build.building", "log.line", "log.line", "push", "readiness.probe"}
	got := codes(tl)
	for i, w := range want[:2] {
		if got[i] != w {
			t.Fatalf("event %d = %q, want %q (all: %v)", i, got[i], w, got)
		}
	}
	for i := 1; i < len(tl.Events); i++ {
		if tl.Events[i].At.Before(tl.Events[i-1].At) {
			t.Fatalf("events out of order at %d: %v", i, got)
		}
	}
	if got[len(got)-1] != "outcome.succeeded" {
		t.Fatalf("last event = %q, want outcome.succeeded", got[len(got)-1])
	}
	if tl.Verdict != VerdictSucceeded || tl.Live {
		t.Fatalf("verdict=%s live=%v", tl.Verdict, tl.Live)
	}
	for _, c := range got {
		if c == "step.image_pull_layer" || c == "step.deploy_summary" {
			t.Fatalf("noise step %q should be dropped", c)
		}
	}
}

func TestAssembleTriggerDetail(t *testing.T) {
	in := baseInputs()
	in.Actor, in.ActorKind = "ci-token", "api"
	tl := Assemble(in)
	trig := tl.Events[0]
	if trig.Trigger == nil || trig.Trigger.Actor != "ci-token" || trig.Trigger.CommitMessage != "fix login" {
		t.Fatalf("trigger = %+v", trig.Trigger)
	}
	if trig.Detail != "main abcdef1 | fix login | by ci-token" {
		t.Fatalf("detail = %q", trig.Detail)
	}
}

func TestGateEventsSupersedeTraceSteps(t *testing.T) {
	in := baseInputs()
	in.Gate = []store.DeployGateEvent{
		{ID: 1, Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, At: at(47), Message: "cut over"},
		{ID: 2, Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusFailed, Reason: deploysafety.ReasonWatchCrashloop, At: at(80), Message: "3 restarts"},
		{ID: 3, Phase: deploysafety.PhaseRollback, Status: deploysafety.StatusPassed, Reason: deploysafety.ReasonAutoRolledBack, At: at(90)},
		{ID: 4, Phase: deploysafety.PhasePin, Status: deploysafety.StatusInfo, At: at(91)},
	}
	tl := Assemble(in)
	count := map[string]int{}
	for _, c := range codes(tl) {
		count[c]++
	}
	if count["step.cutover"] != 0 || count["cutover.passed"] != 1 {
		t.Fatalf("cutover not deduplicated: %v", count)
	}
	if count["watch.crashloop"] != 1 || count["rollback.auto"] != 1 {
		t.Fatalf("watch signals missing: %v", count)
	}
	if tl.Verdict != VerdictRolledBack {
		t.Fatalf("verdict = %s", tl.Verdict)
	}
}

func TestWatchSignalCodes(t *testing.T) {
	cases := map[string]string{
		deploysafety.ReasonWatchOOMKilled: "watch.oom",
		deploysafety.ReasonWatchErrorRate: "watch.error_rate",
		deploysafety.ReasonWatchLatency:   "watch.latency",
		deploysafety.ReasonWatchCrashloop: "watch.crashloop",
		deploysafety.ReasonWatchExited:    "watch.exited",
	}
	for reason, want := range cases {
		got := watchCode(store.DeployGateEvent{Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusFailed, Reason: reason})
		if got != want {
			t.Errorf("%s: got %s want %s", reason, got, want)
		}
	}
}

func TestLogCapKeepsTail(t *testing.T) {
	in := baseInputs()
	in.Config.MaxLogLines = 2
	in.Logs = nil
	for i := 0; i < 5; i++ {
		in.Logs = append(in.Logs, LogLine{At: at(5 + i), Message: "line"})
	}
	tl := Assemble(in)
	if tl.LogsDropped != 3 {
		t.Fatalf("dropped = %d, want 3", tl.LogsDropped)
	}
	n := 0
	for _, e := range tl.Events {
		if e.Kind == KindLog {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("log events = %d", n)
	}
}

func TestInFlightDeployIsLive(t *testing.T) {
	in := baseInputs()
	in.Attempt.FinishedAt = nil
	in.Attempt.Status = store.DeployAttemptStatusRunning
	tl := Assemble(in)
	if !tl.Live || tl.Verdict != VerdictRunning {
		t.Fatalf("live=%v verdict=%s", tl.Live, tl.Verdict)
	}
	for _, e := range tl.Events {
		if e.Kind == KindOutcome {
			t.Fatal("unfinished deploy must not have an outcome")
		}
	}
}

func TestWatchingVerdictWhileWatchOpen(t *testing.T) {
	in := baseInputs()
	in.WatchOpen = true
	tl := Assemble(in)
	if tl.Verdict != VerdictWatching || !tl.Live {
		t.Fatalf("verdict=%s live=%v", tl.Verdict, tl.Live)
	}
}

func TestHooksOutsideTheDeployAreIgnored(t *testing.T) {
	in := baseInputs()
	in.Hooks = []store.HookRun{
		{HookType: "pre_deploy", Command: "migrate", Success: true, RanAt: at(10)},
		{HookType: "post_deploy", Command: "old", Success: true, RanAt: t0.Add(-time.Hour)},
	}
	tl := Assemble(in)
	count := 0
	for _, e := range tl.Events {
		if e.Kind == KindHook {
			count++
			if e.Code != "hook.pre_deploy" {
				t.Fatalf("code = %s", e.Code)
			}
		}
	}
	if count != 1 {
		t.Fatalf("hook events = %d", count)
	}
}

func TestFailedProbeEvent(t *testing.T) {
	e := probeEvent(store.ProbeAttempt{ID: 7, Target: "10.0.0.5:8080", StatusCode: 503, LatencyMS: 12, Error: "unhealthy", ProbedAt: at(3)})
	if e.Status != StatusFailed || e.ID != "probe:7" {
		t.Fatalf("event = %+v", e)
	}
}

func TestImpactEventPlacement(t *testing.T) {
	in := baseInputs()
	im := Impact{CutoverAt: at(47), WindowMS: 60000, Complete: true}
	in.Impact = &im
	tl := Assemble(in)
	var got *Event
	for i := range tl.Events {
		if tl.Events[i].Kind == KindTraffic {
			got = &tl.Events[i]
		}
	}
	if got == nil || !got.At.Equal(at(107)) {
		t.Fatalf("impact event = %+v", got)
	}
	im.Complete = false
	partial := impactEvent(im, at(70))
	if !partial.At.Equal(at(70)) {
		t.Fatalf("partial impact at %v", partial.At)
	}
}

func TestSettled(t *testing.T) {
	in := baseInputs()
	if !Settled(in) {
		t.Fatal("finished deploy with a summary should be settled")
	}
	in.Now = at(80)
	if Settled(in) {
		t.Fatal("impact window still open at cutover+33s")
	}
	in = baseInputs()
	in.WatchOpen = true
	if Settled(in) {
		t.Fatal("open watch is not settled")
	}
	in = baseInputs()
	in.Trace = in.Trace[:2]
	in.Now = at(90)
	if Settled(in) {
		t.Fatal("no summary and within idle window is not settled")
	}
	in.Now = at(200)
	if !Settled(in) {
		t.Fatal("no summary but idle window passed is settled")
	}
	in.Attempt.FinishedAt = nil
	if Settled(in) {
		t.Fatal("running deploy is not settled")
	}
}

func TestSignatureChangesWithContent(t *testing.T) {
	a := Event{ID: "x", At: at(1), Status: StatusRunning}
	b := a
	b.Status = StatusOK
	if a.Signature() == b.Signature() {
		t.Fatal("status change must change signature")
	}
	again := a
	if a.Signature() != again.Signature() {
		t.Fatal("signature must be stable")
	}
}

func TestCompare(t *testing.T) {
	ok := Assemble(baseInputs())
	in := baseInputs()
	in.Gate = []store.DeployGateEvent{{ID: 2, Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusFailed, Reason: deploysafety.ReasonWatchLatency, At: at(80)}}
	in.Trace[0].DurationMS = 59000
	in.Trace[0].EndedAt = at(60)
	bad := Assemble(in)
	c := Compare(ok, bad)
	if len(c.OnlyB) != 1 || c.OnlyB[0] != "watch.latency" || len(c.OnlyA) != 0 {
		t.Fatalf("only a=%v b=%v", c.OnlyA, c.OnlyB)
	}
	var build *PhaseDelta
	for i := range c.Phases {
		if c.Phases[i].Phase == PhaseBuild {
			build = &c.Phases[i]
		}
	}
	if build == nil || build.DeltaMS == nil || *build.DeltaMS <= 0 {
		t.Fatalf("build delta = %+v", build)
	}
}

func TestImpactChange(t *testing.T) {
	a := &Impact{Before: Window{P95Ms: 100}, After: Window{P95Ms: 120}}
	b := &Impact{Before: Window{P95Ms: 100}, After: Window{P95Ms: 300}}
	d := impactChange(a, b)
	if d == nil || d.P95Ms != 180 {
		t.Fatalf("delta = %+v", d)
	}
	if impactChange(a, nil) != nil {
		t.Fatal("missing side gives no delta")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvRetention, "48h")
	t.Setenv(EnvMaxLogLines, "7")
	t.Setenv(EnvImpactWindow, "bogus")
	c := ConfigFromEnv()
	if c.Retention != 48*time.Hour || c.MaxLogLines != 7 || c.ImpactWindow != defaultImpactWindow {
		t.Fatalf("config = %+v", c)
	}
}

func TestTriggerPrecedesStepsRecordedBeforeTheAttemptRow(t *testing.T) {
	in := baseInputs()
	in.Trace = append(in.Trace, store.DeployTraceStep{Key: "resolve_digest", Step: "resolve_digest", Status: store.TraceStatusDone, StartedAt: at(-3), EndedAt: at(-2)})
	tl := Assemble(in)
	if tl.Events[0].Code != "trigger.git_push" || !tl.Events[0].At.Equal(at(-3)) {
		t.Fatalf("first event = %s at %v", tl.Events[0].Code, tl.Events[0].At)
	}
}

func TestOutcomeWaitsForTheReconcilerAfterHandoff(t *testing.T) {
	in := baseInputs()
	in.Attempt.FinishedAt = finished(1)
	tl := Assemble(in)
	last := tl.Events[len(tl.Events)-1]
	if last.Code != "outcome.succeeded" || !last.At.Equal(at(59)) {
		t.Fatalf("last = %s at %v, want the outcome at the summary time", last.Code, last.At)
	}
}

func TestRolledBackOutcomeLandsAfterTheRollback(t *testing.T) {
	in := baseInputs()
	in.Gate = []store.DeployGateEvent{{ID: 3, Phase: deploysafety.PhaseRollback, Status: deploysafety.StatusPassed, At: at(300)}}
	tl := Assemble(in)
	last := tl.Events[len(tl.Events)-1]
	if last.Code != "outcome.rolled_back" || !last.At.Equal(at(300)) {
		t.Fatalf("last = %s at %v", last.Code, last.At)
	}
}
