package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
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

	defaultHookTimeout = 60 * time.Second
	defaultHookCap     = 600 * time.Second
	defaultHookOutput  = 64 << 10
)

// ErrInconsistent marks a backup taken after the pre-hook failed or timed out.
var ErrInconsistent = errors.New("inconsistent")

// ContainerQuiescer is the real Quiescer: hooks run with sh -c inside the
// app's container, pause freezes it for the duration of the archive.
type ContainerQuiescer struct {
	Resolve AppContainerResolver
	// Timeout bounds one hook; MaxTimeout is the hard cap on Timeout.
	Timeout    time.Duration
	MaxTimeout time.Duration
	// MaxOutput caps the bytes read from one hook.
	MaxOutput int64
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
}

func (q *ContainerQuiescer) hookTimeout() time.Duration {
	t, limit := q.Timeout, q.MaxTimeout
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

// Begin implements Quiescer. A failed or timed out pre-hook does not stop
// the backup: it returns a usable end function plus an ErrInconsistent error
// so the caller snapshots anyway, flags it, and still runs the post-hook.
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
				errs = append(errs, fmt.Errorf("post-backup hook: %w", err))
			}
		}
		return errors.Join(errs...)
	}
	var preErr error
	if p.PreHook != "" {
		if err := q.runHook(ctx, rt, id, serviceName, "pre", p.PreHook); err != nil {
			preErr = fmt.Errorf("%w: pre-backup hook: %w", ErrInconsistent, err)
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
	return end, preErr
}

// runHook errors never carry the command or its output: hooks routinely hold
// credentials (an RCON password) and both would land in logs and audit.
func (q *ContainerQuiescer) runHook(ctx context.Context, rt docker.Runtime, containerID, service, phase, command string) error {
	ctx, cancel := context.WithTimeout(ctx, q.hookTimeout())
	defer cancel()
	limit := q.MaxOutput
	if limit <= 0 {
		limit = defaultHookOutput
	}
	start := time.Now()
	err := execDrain(ctx, rt, containerID, command, limit)
	if q.Logger != nil {
		q.Logger.Info("backup: hook finished", slog.String("service", service), slog.String("phase", phase),
			slog.Bool("ok", err == nil), slog.Duration("took", time.Since(start)))
	}
	return err
}

// execDrain returns when the hook ends or ctx expires. After the cap the rest
// is still drained so the exec cannot block on a full pipe.
func execDrain(ctx context.Context, rt docker.Runtime, containerID, command string, limit int64) error {
	done := make(chan error, 1)
	go func() {
		rc, err := rt.Exec(ctx, containerID, []string{"sh", "-c", command})
		if err != nil {
			done <- errors.New("hook could not be started")
			return
		}
		defer func() { _ = rc.Close() }()
		if _, err := io.Copy(io.Discard, io.LimitReader(rc, limit)); err != nil {
			done <- errors.New("hook exited with an error")
			return
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			done <- errors.New("hook exited with an error")
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errors.New("hook timed out")
	}
}
