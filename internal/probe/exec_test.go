package probe

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeExecutor struct {
	code   int
	output string
	err    error
	hang   bool
	gotCmd []string
	gotID  string
}

func (f *fakeExecutor) ExecProbe(ctx context.Context, containerID string, cmd []string) (int, string, error) {
	f.gotCmd, f.gotID = cmd, containerID
	if f.hang {
		<-ctx.Done()
		return 0, "", ctx.Err()
	}
	return f.code, f.output, f.err
}

func TestCheck_Exec(t *testing.T) {
	tests := []struct {
		name      string
		exec      *fakeExecutor
		target    Target
		cmd       []string
		limits    Limits
		wantKind  FailureKind
		wantMsg   string
		wantNoMsg string
	}{
		{name: "exit 0 is healthy", exec: &fakeExecutor{}, target: Target{ContainerID: "c1"}, cmd: []string{"pg_isready", "-U", "app"}},
		{name: "non-zero exit carries output", exec: &fakeExecutor{code: 2, output: "/var/run/postgresql:5432 - no response\n"}, target: Target{ContainerID: "c1"}, cmd: []string{"pg_isready"}, wantKind: FailureExitCode, wantMsg: `exec "pg_isready" exited 2: /var/run/postgresql:5432 - no response`},
		{name: "shell form shows the script", exec: &fakeExecutor{code: 1}, target: Target{ContainerID: "c1"}, cmd: ShellCommand("redis-cli ping | grep PONG"), wantKind: FailureExitCode, wantMsg: `exec "redis-cli ping | grep PONG" exited 1`},
		{name: "long output is truncated", exec: &fakeExecutor{code: 1, output: strings.Repeat("x", 100)}, target: Target{ContainerID: "c1"}, cmd: []string{"check"}, limits: Limits{ExecOutputBytes: 10}, wantKind: FailureExitCode, wantMsg: "xxxxxxxxxx...(truncated)", wantNoMsg: strings.Repeat("x", 11)},
		{name: "exec error", exec: &fakeExecutor{err: errors.New("no such container")}, target: Target{ContainerID: "c1"}, cmd: []string{"check"}, wantKind: FailureExecError, wantMsg: "could not run: no such container"},
		{name: "hung command times out", exec: &fakeExecutor{hang: true}, target: Target{ContainerID: "c1"}, cmd: []string{"sleep", "100"}, wantKind: FailureTimeout, wantMsg: `exec "sleep 100" timed out after 30ms`},
		{name: "no container id", exec: &fakeExecutor{}, cmd: []string{"true"}, wantKind: FailureExecNotReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(nil, tt.exec, tt.limits)
			err := p.Check(context.Background(), tt.target, Config{Exec: tt.cmd, Timeout: 30 * time.Millisecond})
			if tt.wantKind == "" {
				if err != nil {
					t.Fatalf("Check() error = %v", err)
				}
				if tt.exec.gotID != tt.target.ContainerID || strings.Join(tt.exec.gotCmd, " ") != strings.Join(tt.cmd, " ") {
					t.Errorf("executor got %q %v", tt.exec.gotID, tt.exec.gotCmd)
				}
				return
			}
			f := asFailure(t, err)
			if f.Kind != tt.wantKind || !strings.Contains(f.Reason, tt.wantMsg) {
				t.Errorf("failure = %s %q, want kind %s containing %q", f.Kind, f.Reason, tt.wantKind, tt.wantMsg)
			}
			if tt.wantNoMsg != "" && strings.Contains(f.Reason, tt.wantNoMsg) {
				t.Errorf("failure %q should have been truncated", f.Reason)
			}
		})
	}
}

func TestCheck_ExecWithoutExecutor(t *testing.T) {
	err := New(nil, nil, Limits{}).Check(context.Background(), Target{ContainerID: "c1"}, Config{Exec: []string{"true"}})
	if f := asFailure(t, err); f.Kind != FailureExecNotReady {
		t.Errorf("kind = %s, want %s", f.Kind, FailureExecNotReady)
	}
}

func TestWaitReady_ExecRetriesUntilZero(t *testing.T) {
	calls := 0
	exec := execFunc(func() (int, string, error) {
		calls++
		if calls < 3 {
			return 1, "not yet", nil
		}
		return 0, "", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := New(nil, exec, Limits{}).WaitReady(ctx, Target{ContainerID: "c"}, Config{Exec: []string{"check"}, Interval: 10 * time.Millisecond})
	if err != nil || calls != 3 {
		t.Errorf("WaitReady() = %v after %d calls, want nil after 3", err, calls)
	}
}

type execFunc func() (int, string, error)

func (f execFunc) ExecProbe(context.Context, string, []string) (int, string, error) { return f() }
