package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// containerPauser is the optional docker.Runtime capability pause needs.
type containerPauser interface {
	PauseContainer(ctx context.Context, id string) error
	UnpauseContainer(ctx context.Context, id string) error
}

// AppContainerResolver finds the node runtime and running container of an app.
type AppContainerResolver func(ctx context.Context, serviceName string) (docker.Runtime, string, error)

// Hook budgets. The timeout and output cap come from env; the hard cap keeps
// a bad value from holding a game server with saving disabled for long.
const (
	EnvBackupHookTimeoutSec = "APP_BACKUP_HOOK_TIMEOUT_SECONDS"
	EnvBackupHookMaxSec     = "APP_BACKUP_HOOK_MAX_SECONDS"
	EnvBackupHookMaxOutput  = "APP_BACKUP_HOOK_MAX_OUTPUT_BYTES"
	// EnvBackupHookKillGrace is how long past the limit the control plane waits
	// for the in-container kill to end the exec before it reports the hook alive.
	EnvBackupHookKillGrace = "APP_BACKUP_HOOK_KILL_GRACE_SECONDS"

	defaultHookTimeout = 60 * time.Second
	defaultHookCap     = 600 * time.Second
	defaultHookOutput  = 64 << 10
	defaultKillGrace   = 5 * time.Second
	minKillGrace       = 2 * time.Second
)

// ErrPreHookFailed marks a backup aborted because its pre-hook failed or
// timed out; the post-hook has already run when it is returned.
var ErrPreHookFailed = errors.New("pre-backup hook failed")

// HookError is a failed hook. Error() is generic and safe for history rows and
// logs; Output holds the masked output, only for callers allowed to see hooks.
type HookError struct {
	Phase  string
	Msg    string
	Output string
}

func (e *HookError) Error() string { return e.Msg }

// errHookStillRunning means the exec did not end within the kill grace.
var errHookStillRunning = errors.New("timed out and may still be running")

// HookBudgets returns the live per-hook timeout and output cap; zero values
// fall back to the static budgets.
type HookBudgets func(ctx context.Context) (timeout time.Duration, maxOutput int64)

// ContainerQuiescer is the real Quiescer: hooks run with sh -c inside the
// app's container, pause freezes it for the duration of the archive.
type ContainerQuiescer struct {
	Resolve AppContainerResolver
	// Timeout bounds one hook; MaxTimeout is the hard cap on Timeout.
	Timeout    time.Duration
	MaxTimeout time.Duration
	// MaxOutput caps the bytes read from one hook.
	MaxOutput int64
	// Budgets, when set, reads Timeout and MaxOutput from live settings.
	Budgets HookBudgets
	// KillGrace is the wait past the limit for the in-container kill to land.
	KillGrace time.Duration
	Logger    *slog.Logger
}

// HookBudgetsFromEnv fills the hook budgets from the environment.
func (q *ContainerQuiescer) HookBudgetsFromEnv(lookup func(string) (string, bool)) {
	num := func(k string) int64 {
		v, ok := lookup(k)
		if !ok {
			return 0
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || n <= 0 {
			return 0
		}
		return n
	}
	q.Timeout = time.Duration(num(EnvBackupHookTimeoutSec)) * time.Second
	q.MaxTimeout = time.Duration(num(EnvBackupHookMaxSec)) * time.Second
	q.MaxOutput = num(EnvBackupHookMaxOutput)
	if g := time.Duration(num(EnvBackupHookKillGrace)) * time.Second; g > 0 {
		q.KillGrace = max(g, minKillGrace)
	}
}

func (q *ContainerQuiescer) killGrace() time.Duration {
	if q.KillGrace > 0 {
		return q.KillGrace
	}
	return defaultKillGrace
}

func (q *ContainerQuiescer) hookTimeout() time.Duration {
	return q.clampTimeout(q.Timeout)
}

func (q *ContainerQuiescer) budgets(ctx context.Context) (time.Duration, int64) {
	t, out := q.Timeout, q.MaxOutput
	if q.Budgets != nil {
		if lt, lo := q.Budgets(ctx); lt > 0 || lo > 0 {
			if lt > 0 {
				t = lt
			}
			if lo > 0 {
				out = lo
			}
		}
	}
	if out <= 0 {
		out = defaultHookOutput
	}
	return q.clampTimeout(t), out
}

func (q *ContainerQuiescer) clampTimeout(t time.Duration) time.Duration {
	limit := q.MaxTimeout
	if t <= 0 {
		t = defaultHookTimeout
	}
	if limit <= 0 {
		limit = defaultHookCap
	}
	if t > limit {
		t = limit
	}
	return t
}

// Begin implements Quiescer. A failed or timed out pre-hook aborts the backup:
// the post-hook runs first, so a pre-hook that froze writes never leaves the
// app frozen, and the error wraps ErrPreHookFailed.
func (q *ContainerQuiescer) Begin(ctx context.Context, serviceName string, p store.VolumeBackupPolicy) (func(context.Context) error, error) {
	rt, id, err := q.Resolve(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("find running container of %q: %w", serviceName, err)
	}
	var pauser containerPauser
	end := func(ctx context.Context) error {
		var errs []error
		if pauser != nil {
			if err := pauser.UnpauseContainer(ctx, id); err != nil {
				errs = append(errs, err)
			}
		}
		if p.PostHook != "" {
			if err := q.runHook(ctx, rt, id, serviceName, "post", p.PostHook); err != nil {
				errs = append(errs, fmt.Errorf("post-backup hook failed: %w", err))
			}
		}
		return errors.Join(errs...)
	}
	if p.PreHook != "" {
		if err := q.runHook(ctx, rt, id, serviceName, "pre", p.PreHook); err != nil {
			preErr := fmt.Errorf("%w: %w", ErrPreHookFailed, err)
			if eerr := end(context.WithoutCancel(ctx)); eerr != nil {
				preErr = errors.Join(preErr, eerr)
			}
			return nil, preErr
		}
	}
	if p.Quiesce == store.VolumeQuiescePause {
		pp, ok := rt.(containerPauser)
		if !ok {
			_ = end(context.WithoutCancel(ctx))
			return nil, errors.New("pausing the app is not supported on this node")
		}
		if err := pp.PauseContainer(ctx, id); err != nil {
			_ = end(context.WithoutCancel(ctx))
			return nil, err
		}
		pauser = pp
	}
	return end, nil
}

// runHook errors never carry the command, and their text never carries the
// output: hooks routinely hold credentials (an RCON password) and the error
// text lands in history rows readers can see and in logs.
func (q *ContainerQuiescer) runHook(ctx context.Context, rt docker.Runtime, containerID, service, phase, command string) error {
	timeout, limit := q.budgets(ctx)
	// The container kills the hook at timeout; the wait runs a grace longer so
	// a clean end proves the kill landed before the post-hook starts.
	hctx, cancel := context.WithTimeout(ctx, timeout+q.killGrace())
	defer cancel()
	start := time.Now()
	out, err := execDrain(hctx, rt, containerID, hookArgv(command, timeout), limit)
	took := time.Since(start)
	if q.Logger != nil {
		q.Logger.Info("backup: hook finished", slog.String("service", service), slog.String("phase", phase),
			slog.Bool("ok", err == nil), slog.Duration("took", took))
	}
	if err == nil {
		return nil
	}
	he := &HookError{Phase: phase, Msg: err.Error(), Output: redactHookOutput(out, command)}
	if !errors.Is(err, errHookStillRunning) && took >= timeout {
		he.Msg = "timed out"
	}
	return he
}

// hookWrapper runs the hook in its own session when setsid exists and kills
// the whole process group at the limit, polling each second so nothing of the
// watchdog outlives the hook by more than a second.
const hookWrapper = `t="$1"; c="$2"
if command -v setsid >/dev/null 2>&1; then setsid sh -c "$c" & else sh -c "$c" & fi
p=$!
( i=0; while [ "$i" -lt "$t" ]; do sleep 1; kill -0 "$p" 2>/dev/null || exit 0; i=$((i+1)); done
  kill -KILL -"$p" 2>/dev/null || kill -KILL "$p" 2>/dev/null ) >/dev/null 2>&1 &
wait "$p"; s=$?
exit "$s"`

// hookArgv passes the command as a positional argument, never spliced into
// the wrapper script, so no quoting of the stored command is needed.
func hookArgv(command string, timeout time.Duration) []string {
	secs := int64(timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	return []string{"sh", "-c", hookWrapper, "levelrail-hook", strconv.FormatInt(secs, 10), command}
}

// redactHookOutput keeps a stored copy of hook output safer: the command text
// is cut out and known secret shapes are masked. It cannot catch a secret the
// hook expanded at runtime, which is why the output is gated separately.
func redactHookOutput(out []byte, command string) string {
	s := strings.TrimSpace(strings.ToValidUTF8(string(out), "?"))
	if s == "" {
		return ""
	}
	if c := strings.TrimSpace(command); c != "" {
		s = strings.ReplaceAll(s, c, "[hook command]")
		for _, f := range strings.Fields(c) {
			if len(f) >= 6 {
				s = strings.ReplaceAll(s, f, "***")
			}
		}
	}
	return telemetry.RedactLine(secretmask.Mask(s))
}

// hookExitError turns an exec failure into text without the command or stderr.
func hookExitError(err error) error {
	var ee *docker.ExecExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("exit %d", ee.ExitCode)
	}
	return errors.New("exited with an error")
}

// execDrain returns when the hook ends or ctx expires, with up to limit bytes
// of output. After the cap the rest is drained so the exec cannot block.
func execDrain(ctx context.Context, rt docker.Runtime, containerID string, argv []string, limit int64) ([]byte, error) {
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		rc, err := rt.Exec(ctx, containerID, argv)
		if err != nil {
			done <- result{err: errors.New("could not be started")}
			return
		}
		defer func() { _ = rc.Close() }()
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, io.LimitReader(rc, limit)); err != nil {
			done <- result{out: buf.Bytes(), err: hookExitError(err)}
			return
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			done <- result{out: buf.Bytes(), err: hookExitError(err)}
			return
		}
		done <- result{}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-ctx.Done():
		// The caller's cancel tears the exec stream down, which ends the goroutine.
		return nil, errHookStillRunning
	}
}
