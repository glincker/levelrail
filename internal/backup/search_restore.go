package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const defaultSearchRestoreTimeout = 30 * time.Minute

// searchUploadScript writes the artifact next to the data, validates it as
// an archive and only then marks it pending, so a truncated upload is never
// loaded. Typesense has no import flag: its snapshot is extracted up front.
func searchUploadScript(engine string, l database.SearchLayout) []string {
	upload := l.VolumePath + "/.restore-upload"
	var b strings.Builder
	b.WriteString(`set -e; U="$0"; P="$1"; S="$2"; rm -rf "$U" "$P" "$S"; cat > "$U"; `)
	switch engine {
	case store.EngineMeilisearch:
		b.WriteString(`tar -tzf "$U" > /dev/null; mv "$U" "$P"`)
	case store.EngineQdrant:
		b.WriteString(`tar -tf "$U" > /dev/null; mv "$U" "$P"`)
	default:
		b.WriteString(`mkdir -p "$S"; tar -xf "$U" -C "$S"; rm -f "$U"; [ -n "$(ls -A "$S")" ]; : > "$P"`)
	}
	return []string{"sh", "-c", b.String(), upload, l.PendingPath(), l.StagingPath()}
}

func (r *ContainerRestorer) searchRestoreTimeout() (time.Duration, time.Duration) {
	timeout, interval := r.ReadyTimeout, r.ReadyInterval
	if timeout <= 0 {
		timeout = defaultSearchRestoreTimeout
	}
	if interval <= 0 {
		interval = defaultRestoreReadyInterval
	}
	return timeout, interval
}

// restoreSearch uploads the artifact, restarts the container so its boot
// script loads it, and confirms only once the engine serves from the boot
// that loaded it. Any failure asks the boot script to put the previous data
// back and reports the restore as failed.
func (r *ContainerRestorer) restoreSearch(ctx context.Context, engine, containerName string, dump io.Reader) error {
	l, _ := database.SearchEngineLayout(engine)
	if err := r.waitSearchHealthy(ctx, engine, containerName); err != nil {
		return fmt.Errorf("backup: restore %s container %q: %w", engine, containerName, err)
	}

	rc, err := r.Runtime.ExecWithInput(ctx, containerName, searchUploadScript(engine, l), dump)
	if err != nil {
		return fmt.Errorf("backup: restore %s container %q: upload: %w", engine, containerName, err)
	}
	if _, err := io.Copy(io.Discard, rc); err != nil {
		_ = rc.Close()
		return fmt.Errorf("backup: restore %s container %q: upload or validate artifact: %w", engine, containerName, err)
	}
	_ = rc.Close()

	if err := r.restartContainer(ctx, containerName); err != nil {
		if rerr := r.ensureRunning(ctx, containerName); rerr != nil {
			return fmt.Errorf("backup: restore %s container %q: restart to load the snapshot: %w; the snapshot stays staged and loads the next time the container starts", engine, containerName, err)
		}
	}

	if err := r.waitSearchLoaded(ctx, engine, containerName, l); err != nil {
		rbErr := r.rollbackSearch(ctx, containerName, l)
		if rbErr != nil {
			return fmt.Errorf("backup: restore %s container %q: %w; rolling back also failed, the previous data is kept at %s: %w", engine, containerName, err, l.PreviousPath(), rbErr)
		}
		return fmt.Errorf("backup: restore %s container %q: %w; the previous data was put back", engine, containerName, err)
	}

	if _, err := execCapture(ctx, r.Runtime, containerName, []string{"sh", "-c", l.ConfirmRestoreScript()}); err != nil {
		return fmt.Errorf("backup: restore %s container %q: loaded but could not be confirmed, the next restart keeps it: %w", engine, containerName, err)
	}
	return nil
}

func (r *ContainerRestorer) restartContainer(ctx context.Context, containerName string) error {
	state, err := r.Runtime.InspectByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	if state == nil {
		return errors.New("container not found")
	}
	if state.Running {
		if err := r.Runtime.Stop(ctx, state.ID, redisStopTimeout); err != nil {
			return fmt.Errorf("stop: %w", err)
		}
	}
	if err := r.Runtime.Start(ctx, state.ID); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return nil
}

// waitSearchHealthy waits for the engine's health endpoint to answer 200.
func (r *ContainerRestorer) waitSearchHealthy(ctx context.Context, engine, containerName string) error {
	timeout, interval := r.searchRestoreTimeout()
	return pollUntil(ctx, timeout, interval, func() (bool, error) {
		err := searchAPI(ctx, r.Runtime, engine, containerName, "GET", searchHealthPath[engine], "", nil)
		return err == nil, err
	})
}

// waitSearchLoaded succeeds once the engine is healthy and the boot log
// holds exactly one boot: the engine that loaded the snapshot is the one
// serving. A second boot means the load crashed and the engine restarted.
func (r *ContainerRestorer) waitSearchLoaded(ctx context.Context, engine, containerName string, l database.SearchLayout) error {
	timeout, interval := r.searchRestoreTimeout()
	return pollUntil(ctx, timeout, interval, func() (bool, error) {
		if err := searchAPI(ctx, r.Runtime, engine, containerName, "GET", searchHealthPath[engine], "", nil); err != nil {
			return false, err
		}
		boots, err := execCapture(ctx, r.Runtime, containerName, []string{"cat", l.BootsPath()})
		if err != nil {
			return false, fmt.Errorf("read boot log: %w", err)
		}
		switch n := strings.Count(boots, "boot"); n {
		case 1:
			return true, nil
		case 0:
			return false, errPermanent{errors.New("the engine started without loading the snapshot")}
		default:
			return false, errPermanent{fmt.Errorf("the engine restarted %d times while loading the snapshot", n-1)}
		}
	})
}

func (r *ContainerRestorer) rollbackSearch(ctx context.Context, containerName string, l database.SearchLayout) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	if err := r.ensureRunning(rctx, containerName); err != nil {
		return err
	}
	err := pollUntil(rctx, time.Minute, time.Second, func() (bool, error) {
		_, err := execCapture(rctx, r.Runtime, containerName, []string{"sh", "-c", l.RequestRollbackScript()})
		return err == nil, err
	})
	if err != nil {
		return fmt.Errorf("request rollback: %w", err)
	}
	if err := r.restartContainer(rctx, containerName); err != nil {
		return fmt.Errorf("restart for rollback: %w", err)
	}
	return nil
}

func (r *ContainerRestorer) ensureRunning(ctx context.Context, containerName string) error {
	state, err := r.Runtime.InspectByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	if state == nil {
		return errors.New("container not found")
	}
	if !state.Running {
		if err := r.Runtime.Start(ctx, state.ID); err != nil {
			return fmt.Errorf("start: %w", err)
		}
	}
	return nil
}

// errPermanent stops pollUntil early: waiting longer cannot fix it.
type errPermanent struct{ err error }

func (e errPermanent) Error() string { return e.err.Error() }
func (e errPermanent) Unwrap() error { return e.err }

// pollUntil retries check every interval until it reports done, returns
// errPermanent, or timeout passes; the last error explains a timeout.
func pollUntil(ctx context.Context, timeout, interval time.Duration, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := check()
		if done {
			return nil
		}
		var perm errPermanent
		if errors.As(err, &perm) {
			return perm.err
		}
		if !time.Now().Before(deadline) {
			if err == nil {
				err = errors.New("condition not met")
			}
			return fmt.Errorf("not ready after %s: %w", timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
