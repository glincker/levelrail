package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const testRconSecret = "hunter2-rcon"

// hookRuntime records every hook in order and fails or hangs chosen ones.
type hookRuntime struct {
	*fakeExecRuntime
	mu     sync.Mutex
	events *[]string
	fail   map[string]bool
	hang   map[string]bool
	// failOut hooks print out and then exit non-zero.
	failOut map[string]bool
	// killed hooks honour the in-container limit: they end at argv's seconds.
	killed map[string]bool
	// block hooks wait on this channel.
	block chan struct{}
	out   string
}

func (h *hookRuntime) Exec(ctx context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	c := cmd[len(cmd)-1]
	h.mu.Lock()
	*h.events = append(*h.events, "hook:"+strings.Fields(c)[0])
	h.mu.Unlock()
	if h.hang[c] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if h.killed[c] {
		secs, _ := strconv.Atoi(cmd[4])
		select {
		case <-time.After(time.Duration(secs) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		h.mu.Lock()
		*h.events = append(*h.events, "killed")
		h.mu.Unlock()
		return io.NopCloser(errReader{err: &docker.ExecExitError{ExitCode: 137, Stderr: c}}), nil
	}
	if h.block != nil && strings.HasPrefix(c, "block") {
		<-h.block
	}
	if h.failOut[c] {
		return io.NopCloser(io.MultiReader(strings.NewReader(h.out), errReader{err: &docker.ExecExitError{ExitCode: 3, Stderr: c}})), nil
	}
	if h.fail[c] {
		return nil, errors.New("exec failed with " + testRconSecret + " in " + c)
	}
	return io.NopCloser(strings.NewReader(h.out)), nil
}

func newHookQuiescer(events *[]string, rt *hookRuntime, buf *strings.Builder) *ContainerQuiescer {
	rt.fakeExecRuntime = &fakeExecRuntime{}
	rt.events = events
	return &ContainerQuiescer{
		Resolve: func(context.Context, string) (docker.Runtime, string, error) { return rt, "cid", nil },
		Timeout: 50 * time.Millisecond, MaxTimeout: 5 * time.Second, KillGrace: 50 * time.Millisecond,
		Logger: slog.New(slog.NewTextHandler(buf, nil)),
	}
}

type orderArchiver struct {
	inner  VolumeArchiver
	events *[]string
}

func (o orderArchiver) Archive(ctx context.Context, v string) (io.ReadCloser, error) {
	*o.events = append(*o.events, "snapshot")
	return o.inner.Archive(ctx, v)
}

func runHooked(t *testing.T, rt *hookRuntime, policy store.VolumeBackupPolicy) (*volTestEnv, []string, string, error) {
	t.Helper()
	var events []string
	var logs strings.Builder
	e := newVolTestEnv(t, Opts{})
	e.runner.VolumeArchiver = orderArchiver{inner: e.vols, events: &events}
	e.runner.Volume.Quiesce = newHookQuiescer(&events, rt, &logs)
	e.hist.policies["web/data"] = policy
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	return e, events, logs.String(), err
}

func TestHooks_SuccessOrder(t *testing.T) {
	e, ev, _, err := runHooked(t, &hookRuntime{}, store.VolumeBackupPolicy{PreHook: "save-off", PostHook: "save-on"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ev, ",") != "hook:save-off,snapshot,hook:save-on" {
		t.Fatalf("events = %v", ev)
	}
	if row := e.hist.rows["bkh_1"]; row.Status != store.BackupStatusSucceeded || row.Error != "" {
		t.Fatalf("row = %+v", row)
	}
}

func TestHooks_NoHooksUnchanged(t *testing.T) {
	_, ev, _, err := runHooked(t, &hookRuntime{}, store.VolumeBackupPolicy{})
	if err != nil || strings.Join(ev, ",") != "snapshot" {
		t.Fatalf("events = %v err = %v", ev, err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// hookOutputHistory records saved hook outputs next to the in-memory rows.
type hookOutputHistory struct {
	*memHistory
	outs map[string]string
}

func (h *hookOutputHistory) SaveBackupHookOutput(_ context.Context, id, phase, out string) error {
	h.outs[id] = phase + ":" + out
	return nil
}

func TestHooks_PreFailureAbortsAndPostRuns(t *testing.T) {
	rt := &hookRuntime{fail: map[string]bool{"save-off " + testRconSecret: true}}
	e, ev, logs, err := runHooked(t, rt, store.VolumeBackupPolicy{PreHook: "save-off " + testRconSecret, PostHook: "save-on " + testRconSecret})
	if !errors.Is(err, ErrPreHookFailed) {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(ev, ",") != "hook:save-off,hook:save-on" {
		t.Fatalf("events = %v", ev)
	}
	row := e.hist.rows["bkh_1"]
	if row.Status != store.BackupStatusFailed || !strings.Contains(row.Error, "pre-backup hook failed") {
		t.Fatalf("row = %+v", row)
	}
	if strings.Contains(row.Error, testRconSecret) || strings.Contains(logs, testRconSecret) {
		t.Fatalf("secret leaked: %q / %q", row.Error, logs)
	}
}

func TestHooks_PreUnkilledReportsStillRunningAndPostRuns(t *testing.T) {
	rt := &hookRuntime{hang: map[string]bool{"save-off": true}}
	e, ev, _, err := runHooked(t, rt, store.VolumeBackupPolicy{PreHook: "save-off", PostHook: "save-on"})
	if !errors.Is(err, ErrPreHookFailed) {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(ev, ",") != "hook:save-off,hook:save-on" {
		t.Fatalf("events = %v", ev)
	}
	if row := e.hist.rows["bkh_1"]; row.Status != store.BackupStatusFailed || !strings.Contains(row.Error, "may still be running") {
		t.Fatalf("row = %+v", row)
	}
}

func TestHooks_PreKilledAtLimitBeforePostRuns(t *testing.T) {
	rt := &hookRuntime{killed: map[string]bool{"save-off": true}}
	var events []string
	var logs strings.Builder
	e := newVolTestEnv(t, Opts{})
	q := newHookQuiescer(&events, rt, &logs)
	q.Timeout, q.KillGrace = time.Second, 700*time.Millisecond
	e.runner.Volume.Quiesce = q
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "save-off", PostHook: "save-on"}
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if !errors.Is(err, ErrPreHookFailed) {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(events, ",") != "hook:save-off,killed,hook:save-on" {
		t.Fatalf("post must start only after the kill landed: %v", events)
	}
	if row := e.hist.rows["bkh_1"]; row.Error != "prepare volume for backup: pre-backup hook failed: timed out" {
		t.Fatalf("row = %+v", row)
	}
}

func TestHooks_ConcurrentRunOfSameVolumeRejected(t *testing.T) {
	block := make(chan struct{})
	rt := &hookRuntime{block: block}
	var events []string
	var logs strings.Builder
	e := newVolTestEnv(t, Opts{})
	q := newHookQuiescer(&events, rt, &logs)
	q.Timeout = 5 * time.Second
	e.runner.Volume.Quiesce = q
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "block-writes", PostHook: "save-on"}
	first := make(chan error, 1)
	go func() {
		first <- e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !e.runner.VolumeBackupRunning("web", "data") {
		if time.Now().After(deadline) {
			t.Fatal("first run never took the lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_2", "web", "data", "app-web-data", "bkt_test", "")
	if !errors.Is(err, ErrVolumeBackupBusy) {
		t.Fatalf("second run err = %v", err)
	}
	if row := e.hist.rows["bkh_2"]; row.Status != store.BackupStatusFailed || row.Error != ErrVolumeBackupBusy.Error() {
		t.Fatalf("second row = %+v", row)
	}
	close(block)
	if err := <-first; err != nil {
		t.Fatalf("first run: %v", err)
	}
	if e.runner.VolumeBackupRunning("web", "data") {
		t.Fatal("lock not released")
	}
}

func TestHooks_FailureOutputTruncatedAndRedacted(t *testing.T) {
	cmd := "dump --password " + testRconSecret
	out := "token=" + testRconSecret + " ran " + cmd + " " + strings.Repeat("y", 4096)
	rt := &hookRuntime{failOut: map[string]bool{cmd: true}, out: out}
	var events []string
	var logs strings.Builder
	e := newVolTestEnv(t, Opts{})
	q := newHookQuiescer(&events, rt, &logs)
	q.Budgets = func(context.Context) (time.Duration, int64) { return 0, 128 }
	e.runner.Volume.Quiesce = q
	hs := &hookOutputHistory{memHistory: e.hist, outs: map[string]string{}}
	e.runner.Store = hs
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: cmd}
	err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if !errors.Is(err, ErrPreHookFailed) {
		t.Fatalf("err = %v", err)
	}
	if row := e.hist.rows["bkh_1"]; row.Error != "prepare volume for backup: pre-backup hook failed: exit 3" || strings.Contains(logs.String(), "ran") {
		t.Fatalf("history error and logs must stay generic: %q / %q", row.Error, logs.String())
	}
	saved := hs.outs["bkh_1"]
	if !strings.HasPrefix(saved, "pre:") || strings.Contains(saved, testRconSecret) || strings.Contains(saved, "--password") || !strings.Contains(saved, "ran") {
		t.Fatalf("saved output = %q", saved)
	}
	if len(saved) > 200 {
		t.Fatalf("output not truncated: %d bytes", len(saved))
	}
}

func TestHooks_ArgvKeepsCommandOutOfScript(t *testing.T) {
	argv := hookArgv(`echo "$HOME"; rm -rf /x`, 2500*time.Millisecond)
	if argv[0] != "sh" || argv[2] != hookWrapper || argv[4] != "2" || argv[5] != `echo "$HOME"; rm -rf /x` {
		t.Fatalf("argv = %q", argv)
	}
}

func TestHooks_LiveBudgetsClamped(t *testing.T) {
	q := &ContainerQuiescer{MaxTimeout: 2 * time.Second, Budgets: func(context.Context) (time.Duration, int64) { return time.Hour, 64 }}
	if to, out := q.budgets(context.Background()); to != 2*time.Second || out != 64 {
		t.Fatalf("budgets = %v %d", to, out)
	}
}

func TestHooks_PostRunsWhenBackupFails(t *testing.T) {
	var events []string
	var logs strings.Builder
	e := newVolTestEnv(t, Opts{})
	e.runner.Volume.Quiesce = newHookQuiescer(&events, &hookRuntime{}, &logs)
	e.runner.Volume.MaxBytes = 10
	e.hist.policies["web/data"] = store.VolumeBackupPolicy{PreHook: "save-off", PostHook: "save-on"}
	if err := e.runner.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", ""); err == nil {
		t.Fatal("expected the oversize backup to fail")
	}
	if strings.Join(events, ",") != "hook:save-off,hook:save-on" {
		t.Fatalf("events = %v", events)
	}
}

func TestHooks_PostFailureNeverLeaksSecret(t *testing.T) {
	rt := &hookRuntime{fail: map[string]bool{"save-on " + testRconSecret: true}}
	e, _, logs, err := runHooked(t, rt, store.VolumeBackupPolicy{PostHook: "save-on " + testRconSecret})
	if err == nil {
		t.Fatal("a failing post-hook must surface")
	}
	if strings.Contains(err.Error(), testRconSecret) || strings.Contains(logs, testRconSecret) || strings.Contains(e.hist.rows["bkh_1"].Error, testRconSecret) {
		t.Fatalf("secret leaked: %v / %q", err, logs)
	}
}

func TestHooks_TimeoutClampedToHardCap(t *testing.T) {
	q := &ContainerQuiescer{Timeout: time.Hour, MaxTimeout: 2 * time.Second}
	if q.hookTimeout() != 2*time.Second {
		t.Fatalf("timeout = %v", q.hookTimeout())
	}
	q.HookBudgetsFromEnv(func(k string) (string, bool) {
		return map[string]string{EnvBackupHookTimeoutSec: "5", EnvBackupHookMaxSec: "3", EnvBackupHookMaxOutput: "128"}[k], true
	})
	if q.hookTimeout() != 3*time.Second || q.MaxOutput != 128 {
		t.Fatalf("budgets = %v %d", q.hookTimeout(), q.MaxOutput)
	}
}

func TestHooks_OutputCappedNotBlocking(t *testing.T) {
	rt := &hookRuntime{out: strings.Repeat("x", 1<<20)}
	_, ev, _, err := runHooked(t, rt, store.VolumeBackupPolicy{PreHook: "noisy"})
	if err != nil || len(ev) != 2 {
		t.Fatalf("events = %v err = %v", ev, err)
	}
}
