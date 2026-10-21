// Package scheduledtask runs operator-defined jobs on a cron schedule, in
// the running container (exec mode) or in a fresh one-off container built
// from the app's own spec (fresh mode), mirroring internal/backup's
// Runner/Scheduler split: Runner (this file) does one run and records its
// outcome, Scheduler (scheduler.go) decides when a run is due. The same
// Runner backs the cron loop, "run now" and the API trigger, so there is
// one place that executes a command and records what happened.
package scheduledtask

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/backoff"
)

// defaultRunTimeout bounds one attempt when neither the job nor
// APP_JOBS_DEFAULT_TIMEOUT sets a timeout.
const defaultRunTimeout = 10 * time.Minute

// maxOutputBytes caps the output kept per run: the tail, since the last
// lines of a failing script are usually the error.
const maxOutputBytes = 64 * 1024

const redactedMarker = envredact.Marker

// AppStore resolves a job's target service.
type AppStore interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// RunStore persists a run's outcome onto the task's own row.
type RunStore interface {
	RecordScheduledTaskRun(ctx context.Context, id string, ranAt time.Time, status, output string) error
}

// HistoryStore persists per-run history. Optional on Runner: nil keeps only
// the last-run snapshot RunStore writes.
type HistoryStore interface {
	CreateScheduledTaskRun(ctx context.Context, run store.ScheduledTaskRun) error
	FinishScheduledTaskRun(ctx context.Context, run store.ScheduledTaskRun) error
	PruneScheduledTaskRuns(ctx context.Context, taskID string, keep int, outputCutoff time.Time) error
}

// SpecBuilder derives a one-off container spec from a service, so a fresh
// job gets the service's resolved env, volumes and network. Optional:
// without it fresh mode reports itself unavailable.
type SpecBuilder interface {
	BuildJobSpec(ctx context.Context, svc *store.DesiredService, rt docker.Runtime, name string, command []string, res *docker.Resources) (application.JobSpec, error)
	JobSecretValues(ctx context.Context, svc *store.DesiredService, rt docker.Runtime) ([]string, error)
}

// NodeRuntimeResolver resolves the docker.Runtime for a node ID, the same
// shape internal/api's own resolver has.
type NodeRuntimeResolver func(nodeID string) (docker.Runtime, error)

// Trigger says why a run happens and, for a scheduled one, which cron
// occurrence it was claimed for.
type Trigger struct {
	Kind string
	// RunID is the pre-claimed history row, set by Scheduler; empty makes
	// the runner create one.
	RunID        string
	ScheduledFor *time.Time
	CatchUp      bool
}

// Runner runs one job and records the outcome.
type Runner struct {
	Apps     AppStore
	Runs     RunStore
	History  HistoryStore
	Specs    SpecBuilder
	Resolver NodeRuntimeResolver
	Logger   *slog.Logger
	Limits   Limits
	// Target and HTTPConfigs serve http-mode jobs; Client overrides the
	// HTTP client (nil uses http.DefaultClient).
	Target      HTTPTarget
	HTTPConfigs HTTPConfigStore
	Client      *http.Client
	// NamePrefix namespaces fresh job containers (the brand short name).
	NamePrefix string

	// Now returns the current time; nil falls back to time.Now.
	Now func() time.Time
	// Timeout overrides every attempt's timeout when positive; a test seam.
	Timeout time.Duration
	// Sleep waits between retries; nil uses a context-aware timer.
	Sleep func(ctx context.Context, d time.Duration) error

	mu       sync.Mutex
	inFlight map[string]*inFlightRun
	perApp   map[string]int
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) limits() Limits {
	if r.Limits.DefaultTimeout == 0 {
		return DefaultLimits()
	}
	return r.Limits
}

func (r *Runner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func (r *Runner) attemptTimeout(task store.ScheduledTask) time.Duration {
	switch {
	case r.Timeout > 0:
		return r.Timeout
	case task.TimeoutSeconds > 0:
		return time.Duration(task.TimeoutSeconds) * time.Second
	default:
		return r.limits().DefaultTimeout
	}
}

// Run executes task for a manual "run now".
func (r *Runner) Run(ctx context.Context, task store.ScheduledTask) error {
	return r.RunTrigger(ctx, task, Trigger{Kind: store.ScheduledTaskTriggerManual})
}

// Start records a run's history row synchronously, then runs it in the
// background, and returns the run ID so a caller can follow it. Without a
// history store it still runs, with an empty ID. ctx should already be
// detached from the request that asked.
func (r *Runner) Start(ctx context.Context, task store.ScheduledTask, trigger string) (string, error) {
	trig := Trigger{Kind: trigger}
	if r.History != nil {
		id, err := NewRunID()
		if err != nil {
			return "", fmt.Errorf("scheduledtask: start %q: %w", task.ID, err)
		}
		row := store.ScheduledTaskRun{
			ID: id, TaskID: task.ID, ServiceName: task.ServiceName, Trigger: trigger,
			Status: store.ScheduledTaskStatusRunning, StartedAt: r.now(), Mode: modeOf(task),
		}
		if err := r.History.CreateScheduledTaskRun(ctx, row); err != nil {
			return "", fmt.Errorf("scheduledtask: create run for %q: %w", task.ID, err)
		}
		trig.RunID = id
	}
	go func() {
		if err := r.RunTrigger(ctx, task, trig); err != nil {
			r.log().Warn("scheduledtask: on-demand run failed", slog.String("task_id", task.ID), slog.String("error", err.Error()))
		}
	}()
	return trig.RunID, nil
}

// result is one attempt's, then the whole run's, outcome.
type result struct {
	status      string
	output      string
	message     string
	exitCode    *int
	containerID string
	nodeID      string
	attempts    int
	secrets     []string
	infraErr    error
}

// RunTrigger runs task once. task.ConcurrencyPolicy governs a still
// in-flight previous run of the same task (forbid skips, replace cancels
// it), and the per-app cap skips a run when the app already has
// Limits.MaxConcurrentPerApp jobs running. Retries with backoff happen
// inside the one run. The returned error is only an infrastructure
// failure; a failing command is a recorded outcome, not an error.
func (r *Runner) RunTrigger(ctx context.Context, task store.ScheduledTask, trig Trigger) error {
	policy := normalizeConcurrencyPolicy(task.ConcurrencyPolicy)

	runCtx, cleanup, skip := r.beginRun(ctx, task.ID, policy)
	if skip {
		return r.settleWithoutRunning(ctx, task, trig, store.ScheduledTaskStatusSkippedConcurrency,
			"skipped: a previous run of this task is still in flight and concurrency_policy is \"forbid\"")
	}
	defer cleanup()

	if !r.acquireAppSlot(task.ServiceName) {
		return r.settleWithoutRunning(ctx, task, trig, store.ScheduledTaskStatusSkippedConcurrency,
			fmt.Sprintf("skipped: app %q already has %d jobs running (APP_JOBS_MAX_CONCURRENT_PER_APP)", task.ServiceName, r.limits().MaxConcurrentPerApp))
	}
	defer r.releaseAppSlot(task.ServiceName)

	started := r.now()
	runID, err := r.openRun(ctx, task, trig, started)
	if err != nil {
		return err
	}

	res := r.execute(runCtx, task, runID)
	return r.closeRun(ctx, task, trig, runID, started, res)
}

func (r *Runner) acquireAppSlot(service string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perApp == nil {
		r.perApp = make(map[string]int)
	}
	if r.perApp[service] >= r.limits().MaxConcurrentPerApp {
		return false
	}
	r.perApp[service]++
	return true
}

func (r *Runner) releaseAppSlot(service string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perApp[service] > 0 {
		r.perApp[service]--
	}
}

func (r *Runner) openRun(ctx context.Context, task store.ScheduledTask, trig Trigger, started time.Time) (string, error) {
	if trig.RunID != "" || r.History == nil {
		return trig.RunID, nil
	}
	id, err := NewRunID()
	if err != nil {
		return "", fmt.Errorf("scheduledtask: run %q: %w", task.ID, err)
	}
	row := store.ScheduledTaskRun{
		ID: id, TaskID: task.ID, ServiceName: task.ServiceName, Trigger: trig.Kind,
		Status: store.ScheduledTaskStatusRunning, StartedAt: started, Mode: modeOf(task),
	}
	if err := r.History.CreateScheduledTaskRun(ctx, row); err != nil {
		return "", fmt.Errorf("scheduledtask: create run for %q: %w", task.ID, err)
	}
	return id, nil
}

// settleWithoutRunning records a run that never executed (a concurrency
// skip) in both the history and the task snapshot.
func (r *Runner) settleWithoutRunning(ctx context.Context, task store.ScheduledTask, trig Trigger, status, message string) error {
	now := r.now()
	if r.History != nil {
		row := store.ScheduledTaskRun{
			ID: trig.RunID, TaskID: task.ID, ServiceName: task.ServiceName, Trigger: trig.Kind,
			Status: status, ScheduledFor: trig.ScheduledFor, StartedAt: now, FinishedAt: &now,
			Mode: modeOf(task), Message: message,
		}
		var err error
		if trig.RunID != "" {
			err = r.History.FinishScheduledTaskRun(ctx, row)
		} else if row.ID, err = NewRunID(); err == nil {
			err = r.History.CreateScheduledTaskRun(ctx, row)
		}
		if err != nil {
			return fmt.Errorf("scheduledtask: record skipped run for %q: %w", task.ID, err)
		}
	}
	return r.record(ctx, task.ID, status, message)
}

func (r *Runner) closeRun(ctx context.Context, task store.ScheduledTask, trig Trigger, runID string, started time.Time, res result) error {
	ended := r.now()
	output := redact(res.output, res.secrets)
	message := redact(res.message, res.secrets)
	snapshot := output
	if snapshot == "" {
		snapshot = message
	}
	if r.History != nil && runID != "" {
		row := store.ScheduledTaskRun{
			ID: runID, TaskID: task.ID, ServiceName: task.ServiceName, Trigger: trig.Kind,
			Status: res.status, ScheduledFor: trig.ScheduledFor, StartedAt: started, FinishedAt: &ended,
			ExitCode: res.exitCode, DurationMS: ended.Sub(started).Milliseconds(), Attempts: res.attempts,
			NodeID: res.nodeID, Mode: modeOf(task), ContainerID: res.containerID,
			Message: message, Output: capTail(output, r.outputCap()),
		}
		if err := r.History.FinishScheduledTaskRun(ctx, row); err != nil {
			return fmt.Errorf("scheduledtask: finish run %q of %q: %w", runID, task.ID, err)
		}
		l := r.limits()
		if err := r.History.PruneScheduledTaskRuns(ctx, task.ID, l.HistoryKeep, ended.Add(-l.OutputRetention)); err != nil {
			r.log().Warn("scheduledtask: prune history failed", slog.String("task_id", task.ID), slog.String("error", err.Error()))
		}
	}
	if rerr := r.record(ctx, task.ID, res.status, snapshot); rerr != nil {
		return rerr
	}
	return res.infraErr
}

func (r *Runner) outputCap() int {
	if n := r.limits().MaxOutputBytes; n > 0 {
		return n
	}
	return maxOutputBytes
}

func modeOf(task store.ScheduledTask) string {
	switch task.Mode {
	case store.ScheduledTaskModeFresh, store.ScheduledTaskModeHTTP:
		return task.Mode
	}
	return store.ScheduledTaskModeExec
}

// retryable reports whether a failed attempt is worth another try.
func retryable(status string) bool {
	switch status {
	case store.ScheduledTaskStatusFailed, store.ScheduledTaskStatusTimeout, store.ScheduledTaskStatusContainerNotRunning:
		return true
	}
	return false
}

func (r *Runner) execute(runCtx context.Context, task store.ScheduledTask, runID string) result {
	l := r.limits()
	retries := task.MaxRetries
	if retries > l.MaxRetries {
		retries = l.MaxRetries
	}
	if retries < 0 {
		retries = 0
	}

	var res result
	var history []string
	for attempt := 1; ; attempt++ {
		res = r.attempt(runCtx, task, runID)
		res.attempts = attempt
		if res.status == store.ScheduledTaskStatusSuccess || !retryable(res.status) || attempt > retries || runCtx.Err() != nil {
			break
		}
		history = append(history, fmt.Sprintf("attempt %d: %s", attempt, res.status))
		if err := r.sleep(runCtx, r.backoff(task, attempt)); err != nil {
			if isReplaced(runCtx) {
				res = result{status: store.ScheduledTaskStatusReplaced, message: replacedOutputMessage, attempts: attempt, nodeID: res.nodeID}
			}
			break
		}
	}
	if len(history) > 0 && res.message != "" {
		res.message = strings.Join(history, "; ") + "; " + res.message
	} else if len(history) > 0 {
		res.message = strings.Join(history, "; ")
	}
	return res
}

func (r *Runner) backoff(task store.ScheduledTask, attempt int) time.Duration {
	l := r.limits()
	base := l.RetryBackoff
	if task.RetryBackoffSeconds > 0 {
		base = time.Duration(task.RetryBackoffSeconds) * time.Second
	}
	if l.MaxRetryBackoff <= 0 {
		return base
	}
	return backoff.Exponential(base, l.MaxRetryBackoff, attempt)
}

func (r *Runner) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// attempt performs one try in the task's mode.
func (r *Runner) attempt(runCtx context.Context, task store.ScheduledTask, runID string) result {
	if task.Mode == store.ScheduledTaskModeHTTP {
		return r.attemptHTTP(runCtx, task, runID)
	}
	svc, err := r.Apps.GetDesiredService(runCtx, task.ServiceName)
	if err != nil {
		if isReplaced(runCtx) {
			return result{status: store.ScheduledTaskStatusReplaced, message: replacedOutputMessage}
		}
		return result{
			status: store.ScheduledTaskStatusFailed, message: "load app: " + err.Error(),
			infraErr: fmt.Errorf("scheduledtask: run %q: load service %q: %w", task.ID, task.ServiceName, err),
		}
	}
	res := result{nodeID: svc.NodeID}

	rt, err := r.Resolver(svc.NodeID)
	if err != nil {
		r.log().Warn("scheduledtask: resolve node runtime failed",
			slog.String("task_id", task.ID), slog.String("service", task.ServiceName), slog.String("error", err.Error()))
		res.status, res.output = store.ScheduledTaskStatusContainerNotRunning, "node unreachable: "+err.Error()
		return res
	}

	timeout := r.attemptTimeout(task)
	execCtx, cancel := context.WithTimeout(runCtx, timeout)
	defer cancel()

	if task.Mode == store.ScheduledTaskModeFresh {
		return r.attemptFresh(execCtx, runCtx, task, svc, rt, runID, timeout, res)
	}
	return r.attemptExec(execCtx, runCtx, task, svc, rt, timeout, res)
}

func (r *Runner) attemptExec(execCtx, runCtx context.Context, task store.ScheduledTask, svc *store.DesiredService, rt docker.Runtime, timeout time.Duration, res result) result {
	target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
	state, err := rt.InspectByName(runCtx, target)
	if err != nil {
		if isReplaced(runCtx) {
			res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
			return res
		}
		res.status, res.message = store.ScheduledTaskStatusFailed, "inspect container: "+err.Error()
		res.infraErr = fmt.Errorf("scheduledtask: run %q: inspect container %q: %w", task.ID, target, err)
		return res
	}
	if state == nil || !state.Running {
		res.status, res.output = store.ScheduledTaskStatusContainerNotRunning, "container not running"
		return res
	}
	res.containerID = state.ID
	if r.Specs != nil {
		if secrets, serr := r.Specs.JobSecretValues(execCtx, svc, rt); serr == nil {
			res.secrets = secrets
		}
	}

	rc, err := rt.Exec(execCtx, state.ID, task.Command)
	if err != nil {
		if isReplaced(execCtx) {
			res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
			return res
		}
		res.status, res.output = store.ScheduledTaskStatusFailed, "exec failed: "+err.Error()
		return res
	}

	type outcome struct {
		out *tailCappedWriter
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		capped := newTailCappedWriter(r.outputCap())
		_, readErr := io.Copy(capped, rc)
		done <- outcome{out: capped, err: readErr}
	}()

	select {
	case o := <-done:
		_ = rc.Close()
		return r.execOutcome(res, o.out, o.err)
	case <-execCtx.Done():
		_ = rc.Close()
		if isReplaced(execCtx) {
			res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
			return res
		}
		if errors.Is(execCtx.Err(), context.Canceled) {
			res.status, res.output = store.ScheduledTaskStatusInterrupted, shutdownMessage
			return res
		}
		res.status, res.output = store.ScheduledTaskStatusTimeout, fmt.Sprintf("command timed out after %s", timeout)
		return res
	}
}

const shutdownMessage = "control plane shut down while this run was in flight"

func (r *Runner) execOutcome(res result, out *tailCappedWriter, err error) result {
	if err == nil {
		zero := 0
		res.status, res.output, res.exitCode = store.ScheduledTaskStatusSuccess, out.String(), &zero
		return res
	}
	var execErr *docker.ExecExitError
	if errors.As(err, &execErr) {
		output := out.String()
		if execErr.Stderr != "" {
			output += "\n--- stderr ---\n" + execErr.Stderr
		}
		code := execErr.ExitCode
		res.status, res.output, res.exitCode = store.ScheduledTaskStatusFailed, output, &code
		return res
	}
	res.status, res.output = store.ScheduledTaskStatusFailed, "reading command output failed: "+err.Error()
	return res
}

func (r *Runner) attemptFresh(execCtx, runCtx context.Context, task store.ScheduledTask, svc *store.DesiredService, rt docker.Runtime, runID string, timeout time.Duration, res result) result {
	if r.Specs == nil {
		res.status, res.message = store.ScheduledTaskStatusFailed, "fresh mode is not available on this control plane"
		return res
	}
	if svc.Image == "" || appspec.IsPendingImage(svc.Image) {
		res.status, res.message = store.ScheduledTaskStatusFailed, "the app has no built image yet"
		return res
	}
	name := r.ContainerName(task.ServiceName, runID, task.ID)
	var limits *docker.Resources
	if task.MemoryLimitMB > 0 || task.CPULimitMillis > 0 {
		limits = &docker.Resources{MemoryBytes: int64(task.MemoryLimitMB) << 20, NanoCPUs: int64(task.CPULimitMillis) * 1_000_000}
	}
	built, err := r.Specs.BuildJobSpec(execCtx, svc, rt, name, task.Command, limits)
	if err != nil {
		if isReplaced(runCtx) {
			res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
			return res
		}
		res.status, res.message = store.ScheduledTaskStatusFailed, "build job container: "+err.Error()
		return res
	}
	res.secrets = built.SecretValues

	out, err := docker.RunToCompletionVia(execCtx, rt, built.Spec, r.outputCap())
	res.containerID = out.ContainerID
	res.output = out.Output
	if !out.OutputCaptured && err == nil {
		res.message = "output is not captured for jobs on remote nodes; run in exec mode to capture it"
	}
	switch {
	case err == nil:
	case isReplaced(execCtx):
		res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
		return res
	case errors.Is(err, context.Canceled):
		res.status, res.message = store.ScheduledTaskStatusInterrupted, shutdownMessage
		return res
	case errors.Is(err, context.DeadlineExceeded):
		res.status = store.ScheduledTaskStatusTimeout
		res.message = fmt.Sprintf("command timed out after %s", timeout)
		return res
	default:
		res.status, res.message = store.ScheduledTaskStatusFailed, err.Error()
		return res
	}
	code := out.ExitCode
	res.exitCode = &code
	// A job is a one-shot workload: exit 0 is success, unlike a service.
	if v := deploysafety.ClassifyExit(deploysafety.WorkloadJob, "", deploysafety.ExitObservation{OOMKilled: out.OOMKilled, ExitCode: code}); v.Failed {
		res.status, res.message = store.ScheduledTaskStatusFailed, v.Message
	} else {
		res.status = store.ScheduledTaskStatusSuccess
	}
	return res
}

// ContainerName is the deterministic name of a fresh job container, so a
// restart can find and remove one the previous process left behind. The
// run suffix is deliberately not hex, so no service-container parser takes
// it for a replica.
func (r *Runner) ContainerName(service, runID, taskID string) string {
	suffix := runID
	if suffix == "" {
		suffix = taskID
	}
	prefix := r.NamePrefix
	if prefix != "" {
		prefix += "-"
	}
	return prefix + "job-" + service + "-" + strings.ReplaceAll(suffix, "_", "-")
}

// Cleanup force-removes the containers of runs a previous process left in
// flight. Best effort: a node that is unreachable now is simply skipped.
func (r *Runner) Cleanup(ctx context.Context, runs []store.ScheduledTaskRun) {
	for _, run := range runs {
		if run.Mode != store.ScheduledTaskModeFresh {
			continue
		}
		svc, err := r.Apps.GetDesiredService(ctx, run.ServiceName)
		if err != nil {
			continue
		}
		rt, err := r.Resolver(svc.NodeID)
		if err != nil {
			continue
		}
		name := r.ContainerName(run.ServiceName, run.ID, run.TaskID)
		state, err := rt.InspectByName(ctx, name)
		if err != nil || state == nil {
			continue
		}
		if err := rt.Remove(ctx, state.ID, true); err != nil {
			r.log().Warn("scheduledtask: remove leftover job container failed",
				slog.String("run_id", run.ID), slog.String("container", name), slog.String("error", err.Error()))
		}
	}
}

// replacedOutputMessage is recorded on a run that concurrency_policy
// "replace" cancelled in favor of a newer invocation.
const replacedOutputMessage = "run cancelled: replaced by a newer invocation (concurrency_policy: replace)"

func (r *Runner) record(ctx context.Context, taskID, status, output string) error {
	if err := r.Runs.RecordScheduledTaskRun(ctx, taskID, r.now(), status, capTail(output, maxOutputBytes)); err != nil {
		return fmt.Errorf("scheduledtask: record run %q: %w", taskID, err)
	}
	return nil
}

// redact replaces every secret value found in s.
func redact(s string, secrets []string) string {
	return envredact.Secrets(s, secrets...)
}

// capTail trims s to its last limit bytes.
func capTail(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}

// tailCappedWriter keeps only the most recent limit bytes written to it.
// Every Write reports full success so io.Copy drains the stream to the end
// and reaches the command's real exit status.
type tailCappedWriter struct {
	limit int
	buf   []byte
	total int64
}

func newTailCappedWriter(limit int) *tailCappedWriter {
	return &tailCappedWriter{limit: limit, buf: make([]byte, 0, limit)}
}

func (w *tailCappedWriter) Write(p []byte) (int, error) {
	w.total += int64(len(p))
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.limit {
		trimmed := make([]byte, w.limit)
		copy(trimmed, w.buf[len(w.buf)-w.limit:])
		w.buf = trimmed
	}
	return len(p), nil
}

func (w *tailCappedWriter) truncated() bool { return w.total > int64(w.limit) }

func (w *tailCappedWriter) String() string {
	if w.total > int64(w.limit) {
		return "[output truncated, showing the last " + strconv.Itoa(w.limit) + " bytes]\n" + string(w.buf)
	}
	return string(w.buf)
}

// NewRunID mints a run ID (jr_ plus 9 random bytes, URL-safe base64).
func NewRunID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "jr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
