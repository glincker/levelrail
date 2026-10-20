package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// PublishLocks is the store surface the pipeline needs to hold a build back
// from going live while its app is locked.
type PublishLocks interface {
	GetPublishLock(ctx context.Context, name string) (*store.PublishLock, error)
	SaveHeldBuild(ctx context.Context, deployID string, svc store.DesiredService, at time.Time) error
}

// WithPublishLocks makes builds of a locked app finish without becoming
// live: the would-be desired state is saved as a held build instead.
func WithPublishLocks(l PublishLocks) Option {
	return func(p *Pipeline) { p.locks = l }
}

// holdForLock saves desired as a held build and reports true when the app is
// locked. The lock is read from the store on every call, never cached.
func (p *Pipeline) holdForLock(ctx context.Context, req Request, desired store.DesiredService) (bool, error) {
	if p.locks == nil {
		return false, nil
	}
	lock, err := p.locks.GetPublishLock(ctx, req.ServiceName)
	if err != nil {
		return false, fmt.Errorf("check publish lock: %w", err)
	}
	if lock == nil {
		return false, nil
	}
	id := req.AttemptID
	if id == "" {
		if id, err = store.NewDeployAttemptID(); err != nil {
			return false, fmt.Errorf("mint held build id: %w", err)
		}
	}
	if err := p.locks.SaveHeldBuild(ctx, id, desired, time.Now()); err != nil {
		return false, fmt.Errorf("save held build: %w", err)
	}
	p.logger.Info("deploy: build held by publish lock", slog.String("service", req.ServiceName), slog.String("deploy_id", id))
	return true, nil
}
