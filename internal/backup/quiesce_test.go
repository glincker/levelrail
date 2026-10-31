package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	out    string
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
		Timeout: 50 * time.Millisecond, MaxTimeout: time.Second,
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

func TestHooks_PreFailureStillBacksUpAndFlags(t *testing.T) {
	rt := &hookRuntime{fail: map[string]bool{"save-off " + testRconSecret: true}}
	e, ev, logs, err := runHooked(t, rt, store.VolumeBackupPolicy{PreHook: "save-off " + testRconSecret, PostHook: "save-on " + testRconSecret})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ev, ",") != "hook:save-off,snapshot,hook:save-on" {
		t.Fatalf("events = %v", ev)
	}
	row := e.hist.rows["bkh_1"]
	if row.Status != store.BackupStatusSucceeded || !strings.HasPrefix(row.Error, "inconsistent") {
		t.Fatalf("row = %+v", row)
	}
	if strings.Contains(row.Error, testRconSecret) || strings.Contains(logs, testRconSecret) {
		t.Fatalf("secret leaked: %q / %q", row.Error, logs)
	}
}

func TestHooks_PreTimeoutIsInconsistentAndPostRuns(t *testing.T) {
	rt := &hookRuntime{hang: map[string]bool{"save-off": true}}
	e, ev, _, err := runHooked(t, rt, store.VolumeBackupPolicy{PreHook: "save-off", PostHook: "save-on"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ev, ",") != "hook:save-off,snapshot,hook:save-on" {
		t.Fatalf("events = %v", ev)
	}
	if !strings.Contains(e.hist.rows["bkh_1"].Error, "timed out") {
		t.Fatalf("row = %+v", e.hist.rows["bkh_1"])
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
