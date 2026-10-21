package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StaticHolds is the extra store surface a locked app's static build needs.
type StaticHolds interface {
	SaveHeldStaticBuild(ctx context.Context, deployID string, site store.StaticSite, at time.Time) error
}

// saveStaticSite points the live site at a staged tree, or records it as a
// held build when the app is locked. A lock lookup failure fails closed.
func (p *Pipeline) saveStaticSite(ctx context.Context, req Request, site store.StaticSite) (held bool, err error) {
	if p.locks != nil {
		lock, err := p.locks.GetPublishLock(ctx, req.ServiceName)
		if err != nil {
			return false, fmt.Errorf("check publish lock: %w", err)
		}
		if lock != nil {
			return true, p.holdStaticSite(ctx, req, site)
		}
	}
	return false, p.staticSites.SaveStaticSite(ctx, site)
}

func (p *Pipeline) holdStaticSite(ctx context.Context, req Request, site store.StaticSite) error {
	holds, ok := p.locks.(StaticHolds)
	if !ok {
		return fmt.Errorf("publish lock is on but the store cannot hold static builds")
	}
	id := req.AttemptID
	if id == "" {
		var err error
		if id, err = store.NewDeployAttemptID(); err != nil {
			return fmt.Errorf("mint held build id: %w", err)
		}
	}
	if err := holds.SaveHeldStaticBuild(ctx, id, site, time.Now()); err != nil {
		return fmt.Errorf("save held build: %w", err)
	}
	p.logger.Info("deploy: static build held by publish lock", slog.String("service", req.ServiceName), slog.String("deploy_id", id))
	return nil
}

// finishStaticPublish reports a finished static deploy. A held one only
// records its digest, so a later publish can verify the tree.
func (p *Pipeline) finishStaticPublish(ctx context.Context, req Request, progress func(build.ProgressEvent), st StaticTreeStats, took time.Duration, held bool) {
	if !held {
		emitStatic(progress, stepStaticPublish, fmt.Sprintf("%d files, %d bytes, digest %s", st.Files, st.Bytes, st.Digest), true)
		p.recordStaticPublished(ctx, req, st, took)
		return
	}
	emitStatic(progress, stepStaticPublish, fmt.Sprintf("held by publish lock, not live: %d files, digest %s", st.Files, st.Digest), true)
	if p.staticRecorder == nil || req.AttemptID == "" {
		return
	}
	if err := p.staticRecorder.SetDeployAttemptDigest(ctx, req.AttemptID, st.Digest, store.DigestReasonStaticTree); err != nil {
		p.logger.Warn("deploy: record held static digest failed", slog.String("attempt_id", req.AttemptID), slog.String("error", err.Error()))
	}
}
