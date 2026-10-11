package backup

import (
	"context"
	"errors"
	"fmt"
	"io"

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

// ContainerQuiescer is the real Quiescer: hooks run with sh -c inside the
// app's container, pause freezes it for the duration of the archive.
type ContainerQuiescer struct {
	Resolve AppContainerResolver
}

// Begin implements Quiescer.
func (q *ContainerQuiescer) Begin(ctx context.Context, serviceName string, p store.VolumeBackupPolicy) (func(context.Context) error, error) {
	rt, id, err := q.Resolve(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("find running container of %q: %w", serviceName, err)
	}
	if p.PreHook != "" {
		if err := runHook(ctx, rt, id, p.PreHook); err != nil {
			return nil, fmt.Errorf("pre-backup hook: %w", err)
		}
	}
	var pauser containerPauser
	if p.Quiesce == store.VolumeQuiescePause {
		pp, ok := rt.(containerPauser)
		if !ok {
			return nil, errors.New("pausing the app is not supported on this node")
		}
		if err := pp.PauseContainer(ctx, id); err != nil {
			return nil, err
		}
		pauser = pp
	}
	return func(ctx context.Context) error {
		var errs []error
		if pauser != nil {
			if err := pauser.UnpauseContainer(ctx, id); err != nil {
				errs = append(errs, err)
			}
		}
		if p.PostHook != "" {
			if err := runHook(ctx, rt, id, p.PostHook); err != nil {
				errs = append(errs, fmt.Errorf("post-backup hook: %w", err))
			}
		}
		return errors.Join(errs...)
	}, nil
}

func runHook(ctx context.Context, rt docker.Runtime, containerID, command string) error {
	rc, err := rt.Exec(ctx, containerID, []string{"sh", "-c", command})
	if err != nil {
		return fmt.Errorf("run %q: %w", command, err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("run %q: %w", command, err)
	}
	return nil
}
