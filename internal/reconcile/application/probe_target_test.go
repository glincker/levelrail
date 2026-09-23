package application

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func execReadiness(cmd ...string) *store.ServiceHealth {
	return &store.ServiceHealth{Readiness: &store.ServiceProbe{Exec: cmd, Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}}
}

func TestController_Reconcile_ExecReadiness(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		stderr     string
		execErr    error
		wantStatus reconcile.ConditionStatus
		wantReason string
		wantMsg    string
	}{
		{name: "exit 0 deploys", wantStatus: reconcile.ConditionTrue, wantReason: "Deployed"},
		{name: "non-zero exit names the command and output", exitCode: 2, stderr: "/var/run/postgresql:5432 - no response", wantStatus: reconcile.ConditionFalse, wantReason: "ReadinessFailed", wantMsg: `exec "pg_isready -U app" exited 2: /var/run/postgresql:5432 - no response`},
		{name: "exec transport failure", execErr: errors.New("exec create: container paused"), wantStatus: reconcile.ConditionFalse, wantReason: "ReadinessFailed", wantMsg: "could not run: exec create: container paused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No port at all: an exec probe must still run, unlike an HTTP one.
			rt := newFakeRuntime(0)
			rt.execExitCode, rt.execStderr, rt.execErr = tt.exitCode, tt.stderr, tt.execErr
			desired := &store.DesiredService{Name: "db", Image: "postgres:16", Health: execReadiness("pg_isready", "-U", "app")}
			c := New("db", &fakeStore{svc: desired}, rt, WithReadyBudget(100*time.Millisecond))

			result, _ := c.Reconcile(context.Background())
			cond := conditionOf(t, result)
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason || !strings.Contains(cond.Message, tt.wantMsg) {
				t.Errorf("condition = %+v, want %s/%s containing %q", cond, tt.wantStatus, tt.wantReason, tt.wantMsg)
			}
			if len(rt.execCalls) == 0 || rt.execCalls[0].containerID == "" || strings.Join(rt.execCalls[0].cmd, " ") != "pg_isready -U app" {
				t.Errorf("exec calls = %+v, want pg_isready run inside the new container", rt.execCalls)
			}
		})
	}
}

// TestController_Reconcile_ExecReadiness_HalfSucceeded covers a redeploy
// whose replacement was created and started but never passed its exec
// probe: the old container must keep serving, and a later pass must
// finish the cutover once the probe passes, without recreating anything.
func TestController_Reconcile_ExecReadiness_HalfSucceeded(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.execExitCode = 1
	old := ContainerName("worker", "img:v1", "")
	rt.seed(old, true)
	desired := &store.DesiredService{Name: "worker", Image: "img:v2", Health: execReadiness("/bin/sh", "-c", "test -f /tmp/ready")}
	c := New("worker", &fakeStore{svc: desired}, rt, WithReadyBudget(60*time.Millisecond))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("first Reconcile() error = nil, want the exec readiness failure")
	}
	assertConditionReasonFalse(t, result, "ReadinessFailed")
	if msg := conditionOf(t, result).Message; !strings.Contains(msg, `exec "test -f /tmp/ready" exited 1`) {
		t.Errorf("message = %q, want the failing command named", msg)
	}
	if _, ok := rt.containers[old]; !ok {
		t.Fatal("old container removed while the replacement was unready")
	}
	replacement := ContainerName("worker", "img:v2", "")
	if _, ok := rt.containers[replacement]; !ok {
		t.Fatal("replacement container missing after the half-succeeded pass")
	}
	createsBefore := rt.createCalls

	rt.mu.Lock()
	rt.execExitCode = 0
	rt.mu.Unlock()

	result, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	if rt.createCalls != createsBefore {
		t.Errorf("createCalls went %d -> %d, want the existing replacement reused", createsBefore, rt.createCalls)
	}
	if names := rt.names(); len(names) != 1 || names[0] != replacement {
		t.Errorf("containers = %v, want only %s", names, replacement)
	}
}

func TestController_Reconcile_RedirectNotFollowed_ReasonNamesFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer srv.Close()

	follow := false
	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/health", FollowRedirects: &follow, Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(60*time.Millisecond))

	result, _ := c.Reconcile(context.Background())
	assertConditionReasonFalse(t, result, "ReadinessFailed")
	if msg := conditionOf(t, result).Message; !strings.Contains(msg, "/health returned 302 to /login; set follow_redirects or expected_status") {
		t.Errorf("message = %q, want the redirect and the fix named", msg)
	}
}

func TestController_Reconcile_ExecLiveness_NoPortStillProbed(t *testing.T) {
	rt := newFakeRuntime(0)
	name := ContainerName("worker", "img:v1", "")
	rt.seed(name, true)
	rt.execExitCode = 3
	rt.execStderr = "queue unreachable"
	desired := &store.DesiredService{
		Name: "worker", Image: "img:v1",
		Health: &store.ServiceHealth{Liveness: &store.ServiceProbe{Exec: []string{"healthcheck"}, Failures: 3, Timeout: 50 * time.Millisecond}},
	}
	c := New("worker", &fakeStore{svc: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Reason != "LivenessDegraded" || !strings.Contains(cond.Message, `exec "healthcheck" exited 3: queue unreachable`) {
		t.Errorf("condition = %+v, want LivenessDegraded naming the exec failure", cond)
	}
}

type hangingExecRuntime struct {
	*fakeRuntime
	closed chan struct{}
}

func (h hangingExecRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		<-h.closed
		_ = pw.Close()
	}()
	return closeNotifier{ReadCloser: pr, closed: h.closed}, nil
}

type closeNotifier struct {
	io.ReadCloser
	closed chan struct{}
}

func (c closeNotifier) Close() error {
	close(c.closed)
	return c.ReadCloser.Close()
}

func TestRuntimeExecutor_TimeoutClosesStream(t *testing.T) {
	rt := hangingExecRuntime{fakeRuntime: newFakeRuntime(0), closed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, _, err := runtimeExecutor{runtime: rt}.ExecProbe(ctx, "c1", []string{"sleep", "60"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ExecProbe() error = %v, want the deadline", err)
	}
	select {
	case <-rt.closed:
	case <-time.After(time.Second):
		t.Fatal("exec stream was never closed after the timeout")
	}
}
