package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeHookRunRecorder captures every UpsertHookRun call, the same
// hand-written-fake-not-mocking-framework pattern fakeStore/fakeRuntime
// already establish in controller_test.go.
type fakeHookRunRecorder struct {
	runs []store.HookRun
	err  error
}

func (f *fakeHookRunRecorder) UpsertHookRun(_ context.Context, run store.HookRun) error {
	f.runs = append(f.runs, run)
	return f.err
}

func TestController_Reconcile_PreDeployHook_Success_RunsBeforeCutover(t *testing.T) {
	rt := newFakeRuntime(0)
	recorder := &fakeHookRunRecorder{}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PreDeploy: "rails db:migrate"},
	}
	rt.execOutput = "migrated\n"
	c := New("web", &fakeStore{svc: desired}, rt, WithHookRunRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if len(rt.execCalls) != 1 {
		t.Fatalf("execCalls = %d, want 1", len(rt.execCalls))
	}
	if got := rt.execCalls[0].cmd; len(got) != 3 || got[0] != "sh" || got[1] != "-c" || got[2] != "rails db:migrate" {
		t.Errorf("exec cmd = %v, want [sh -c \"rails db:migrate\"]", got)
	}
	if rt.count() != 1 {
		t.Errorf("container count = %d, want 1 (the hook must not block a successful deploy)", rt.count())
	}
	if len(recorder.runs) != 1 {
		t.Fatalf("recorded runs = %d, want 1", len(recorder.runs))
	}
	run := recorder.runs[0]
	if run.ServiceName != "web" || run.HookType != store.HookTypePreDeploy || !run.Success || run.Output != "migrated\n" {
		t.Errorf("recorded run = %+v, want a successful pre_deploy run for web with output %q", run, "migrated\n")
	}
}

// TestController_Reconcile_PreDeployHook_Failure_BlocksCutover is the
// core safety property this feature exists for: a failing pre-deploy
// hook must never let the new container become the one serving traffic,
// so the old container must survive untouched and the new one must be
// rolled back.
func TestController_Reconcile_PreDeployHook_Failure_BlocksCutover(t *testing.T) {
	rt := newFakeRuntime(0)
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seed(oldTarget, true)

	recorder := &fakeHookRunRecorder{}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
		Hooks: &store.ServiceHooks{PreDeploy: "rails db:migrate"},
	}
	rt.execExitCode = 1
	rt.execStderr = "migration failed: relation does not exist"
	c := New("web", &fakeStore{svc: desired}, rt, WithHookRunRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "PreDeployHookFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=PreDeployHookFailed", cond)
	}

	names := rt.names()
	if len(names) != 1 || names[0] != oldTarget {
		t.Errorf("containers after failed pre-deploy hook = %v, want only the old container %q to survive", names, oldTarget)
	}
	newTarget := ContainerName("web", "img:v2", "")
	if state, _ := rt.InspectByName(context.Background(), newTarget); state != nil {
		t.Errorf("new container %q still exists, want it rolled back after the hook failed", newTarget)
	}

	if len(recorder.runs) != 1 || recorder.runs[0].Success {
		t.Errorf("recorded runs = %+v, want exactly one failed pre_deploy run", recorder.runs)
	}
	if got := recorder.runs[0].ExitCode; got != 1 {
		t.Errorf("recorded exit code = %d, want 1", got)
	}
}

// TestController_Reconcile_PreDeployHook_RunsOnceAcrossReplicas asserts
// the multi-replica semantics this design deliberately chose: a hook
// like a database migration must run exactly once per deploy, never once
// per replica, even though every replica gets a freshly created
// container from the same new image.
func TestController_Reconcile_PreDeployHook_RunsOnceAcrossReplicas(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Replicas: 3,
		Hooks: &store.ServiceHooks{PreDeploy: "migrate"},
	}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.count() != 3 {
		t.Fatalf("container count = %d, want 3", rt.count())
	}
	if len(rt.execCalls) != 1 {
		t.Errorf("execCalls = %d, want 1 (hook must run once per deploy, not once per replica)", len(rt.execCalls))
	}
}

// TestController_Reconcile_PreDeployHook_NotRunOnPlainRestart covers the
// "created is narrower than justDeployed" distinction ensureReplicaRunning's
// own comment documents: a container that merely needed Start (never
// destroyed, e.g. recovering from a crash) is not a new deploy, so a
// configured pre-deploy hook must not re-run.
func TestController_Reconcile_PreDeployHook_NotRunOnPlainRestart(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PreDeploy: "migrate"},
	}
	target := ContainerName("web", "img:v1", "")
	rt.seed(target, false) // exists, stopped: the crash-recovery case
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.execCalls) != 0 {
		t.Errorf("execCalls = %d, want 0 (an ordinary restart of an already-existing container must not re-run the pre-deploy hook)", len(rt.execCalls))
	}
}

func TestController_Reconcile_PostDeployHook_Success(t *testing.T) {
	rt := newFakeRuntime(0)
	recorder := &fakeHookRunRecorder{}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PostDeploy: "notify-slack"},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithHookRunRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if len(rt.execCalls) != 1 {
		t.Fatalf("execCalls = %d, want 1", len(rt.execCalls))
	}
	if len(recorder.runs) != 1 || recorder.runs[0].HookType != store.HookTypePostDeploy || !recorder.runs[0].Success {
		t.Errorf("recorded runs = %+v, want one successful post_deploy run", recorder.runs)
	}
}

// TestController_Reconcile_PostDeployHook_Failure_KeepsStatusTrueButReports
// is this feature's deliberate, documented asymmetry with a pre-deploy
// failure: the deploy already cut over successfully by the time the
// post-deploy hook runs, so a failure there must not tear the healthy
// container back down, only surface loudly (a real error, a non-True-
// staying Reason change) rather than being swallowed, matching the
// RunningStaleCleanupFailed/DeployedMetricRecordFailed precedent already
// established in this file.
func TestController_Reconcile_PostDeployHook_Failure_KeepsStatusTrueButReports(t *testing.T) {
	rt := newFakeRuntime(0)
	recorder := &fakeHookRunRecorder{}
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PostDeploy: "warm-cache"},
	}
	rt.execExitCode = 2
	rt.execStderr = "cache endpoint unreachable"
	c := New("web", &fakeStore{svc: desired}, rt, WithHookRunRecorder(recorder))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition.Status = %v, want True: a failed post-deploy hook must not undo an already-successful cutover", cond.Status)
	}
	if cond.Reason != "PostDeployHookFailed" {
		t.Errorf("condition.Reason = %q, want PostDeployHookFailed", cond.Reason)
	}
	if rt.count() != 1 {
		t.Errorf("container count = %d, want 1: the healthy container must survive a post-deploy hook failure", rt.count())
	}
	if len(recorder.runs) != 1 || recorder.runs[0].Success {
		t.Errorf("recorded runs = %+v, want exactly one failed post_deploy run", recorder.runs)
	}
}

// TestController_Reconcile_HookRunRecorder_NotConfigured_StillGatesDeploy
// asserts WithHookRunRecorder's own doc comment: persistence is optional,
// the hook's actual gating behavior is not.
func TestController_Reconcile_HookRunRecorder_NotConfigured_StillGatesDeploy(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PreDeploy: "migrate"},
	}
	rt.execExitCode = 1
	c := New("web", &fakeStore{svc: desired}, rt) // no WithHookRunRecorder

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Reason != "PreDeployHookFailed" {
		t.Errorf("condition = %+v, want Reason=PreDeployHookFailed even with no recorder configured", cond)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 (rolled back)", rt.count())
	}
}

// TestController_Reconcile_PreDeployHook_TransportFailure covers Exec
// itself failing (the exec session never started), distinct from the
// command running and exiting nonzero: both must block cutover, but this
// path never reaches a real ExecExitError.
func TestController_Reconcile_PreDeployHook_TransportFailure(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Hooks: &store.ServiceHooks{PreDeploy: "migrate"},
	}
	rt.execErr = errors.New("docker daemon unreachable")
	c := New("web", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Reason != "PreDeployHookFailed" {
		t.Errorf("condition = %+v, want Reason=PreDeployHookFailed", cond)
	}
	if rt.count() != 0 {
		t.Errorf("container count = %d, want 0 (rolled back)", rt.count())
	}
}

// TestController_Reconcile_NoHooks_NeverCallsExec is the baseline
// regression check: a service with no Hooks configured must behave
// exactly as it did before this feature existed, never touching Exec at
// all.
func TestController_Reconcile_NoHooks_NeverCallsExec(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.execCalls) != 0 {
		t.Errorf("execCalls = %d, want 0", len(rt.execCalls))
	}
}
