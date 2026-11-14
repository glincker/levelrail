package cloudiac

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestExecRunnerHelperProcess stands in for the container runtime when re-executed by the tests below.
func TestExecRunnerHelperProcess(t *testing.T) {
	switch {
	case slices.Contains(os.Args, "lr-helper-hold"):
		time.Sleep(30 * time.Second)
	case slices.Contains(os.Args, "lr-helper-ok"):
	default:
		t.Skip("helper process only")
	}
}

// helperRunner records every argv and re-executes the test binary instead of a runtime.
func helperRunner(mode string) (ExecRunner, func() [][]string) {
	var mu sync.Mutex
	var calls [][]string
	r := ExecRunner{command: func(ctx context.Context, argv []string) *exec.Cmd {
		mu.Lock()
		calls = append(calls, argv)
		mu.Unlock()
		m := mode
		if argv[1] == "rm" {
			m = "lr-helper-ok"
		}
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecRunnerHelperProcess$", "--", m) //nolint:gosec // the test binary itself
	}}
	return r, func() [][]string { mu.Lock(); defer mu.Unlock(); return slices.Clone(calls) }
}

func cancelSpec() SandboxSpec {
	return SandboxSpec{Runtime: "docker", Image: "img@sha256:" + strings.Repeat("a", 64), Name: "user-chosen", User: "65532:65532", Args: []string{"plan"}}
}

func containerArg(t *testing.T, argv []string) string {
	t.Helper()
	for _, a := range argv {
		if n, ok := strings.CutPrefix(a, "--name="); ok {
			return n
		}
	}
	t.Fatalf("no --name in %v", argv)
	return ""
}

func TestRunnerCancelRemovesNamedContainerOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("interrupt is not deliverable on windows")
	}
	t.Setenv("APP_CLOUD_ENGINE_STOP_GRACE_SECONDS", "1")
	r, calls := helperRunner("lr-helper-hold")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := r.Run(ctx, cancelSpec()); err == nil {
		t.Fatal("cancelled step reported success")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("cancelled step outlived its wait delay")
	}
	got := calls()
	if len(got) != 2 {
		t.Fatalf("calls = %v, want the run and exactly one rm", got)
	}
	name := containerArg(t, got[0])
	if !stepNameRe.MatchString(name) || strings.Contains(name, "user") || !slices.Contains(got[0], "--rm") {
		t.Fatalf("run argv %v: name %q must be generated and --rm kept", got[0], name)
	}
	if want := []string{"docker", "rm", "-f", name}; !slices.Equal(got[1], want) {
		t.Fatalf("cleanup argv = %v, want %v", got[1], want)
	}
}

func TestRunnerNormalEndRunsNoCleanup(t *testing.T) {
	r, calls := helperRunner("lr-helper-ok")
	if _, err := r.Run(context.Background(), cancelSpec()); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 1 {
		t.Fatalf("calls = %v, want only the run", got)
	}
}

func TestRunnerContainerNamesAreFresh(t *testing.T) {
	a, _ := newContainerName()
	b, _ := newContainerName()
	if a == b || !stepNameRe.MatchString(a) || !stepNameRe.MatchString(b) {
		t.Fatalf("names %q %q", a, b)
	}
}
