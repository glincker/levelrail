package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticRollbackStore is what a static rollback needs beyond the plain
// attempt store. *store.DB satisfies it.
type staticRollbackStore interface {
	GetStaticSite(ctx context.Context, name string) (*store.StaticSite, error)
	SaveStaticSite(ctx context.Context, site store.StaticSite) error
	SetDeployAttemptDigest(ctx context.Context, id, digest, reason string) error
	SetDeployAttemptRollout(ctx context.Context, id, state string) error
	RecordGateEvent(ctx context.Context, e store.DeployGateEvent) error
}

// WithStaticRootDir sets the directory static deploys publish under, so a
// rollback can find a past deploy's tree again.
func WithStaticRootDir(dir string) Option {
	return func(rt *Router) { rt.staticRootDir = dir }
}

// isStaticAttempt reports whether a recorded its content as a published tree.
func isStaticAttempt(a store.DeployAttempt) bool {
	return a.DigestReason == store.DigestReasonStaticTree
}

// rollbackStatic re-points the app's static site at the tree target
// published, after checking the tree still hashes to the recorded digest. It
// records a new succeeded attempt so history shows the rollback.
func (rt *Router) rollbackStatic(ctx context.Context, target store.DeployAttempt) (int, string) {
	st, ok := rt.deployAttempts.(staticRollbackStore)
	if !ok || rt.staticRootDir == "" {
		return http.StatusNotImplemented, "static site rollback is not configured on this control plane"
	}
	if target.Status != store.DeployAttemptStatusSucceeded || target.ImageDigest == "" {
		return http.StatusConflict, "only a succeeded static deploy with a recorded content digest can be rolled back to"
	}
	rel := target.ServiceName
	if target.CommitSHA != "" && filepath.IsLocal(target.CommitSHA) {
		rel = filepath.Join(target.ServiceName, target.CommitSHA)
	}
	dir := filepath.Join(rt.staticRootDir, rel)
	if _, err := os.Stat(dir); err != nil {
		return http.StatusGone, "the published files of this deploy are no longer on disk; redeploy from source instead"
	}
	stats, err := deploy.HashStaticTree(dir)
	if err != nil {
		rt.logger.Error("api: static rollback: hash failed", slog.String("error", err.Error()), slog.String("deploy_id", target.ID))
		return http.StatusInternalServerError, "internal error"
	}
	if stats.Digest != target.ImageDigest {
		return http.StatusConflict, "the files on disk no longer match the content this deploy recorded; redeploy from source instead"
	}
	site := store.StaticSite{Name: target.ServiceName, RootDir: dir}
	if cur, err := st.GetStaticSite(ctx, target.ServiceName); err == nil {
		site.Domains = cur.Domains
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: static rollback: load site failed", slog.String("error", err.Error()), slog.String("name", target.ServiceName))
		return http.StatusInternalServerError, "internal error"
	}
	if err := st.SaveStaticSite(ctx, site); err != nil {
		rt.logger.Error("api: static rollback: save site failed", slog.String("error", err.Error()), slog.String("name", target.ServiceName))
		return http.StatusInternalServerError, "internal error"
	}
	if err := rt.recordStaticRollback(ctx, st, target, stats); err != nil {
		rt.logger.Warn("api: static rollback: record attempt failed", slog.String("error", err.Error()), slog.String("name", target.ServiceName))
	}
	rt.nudgeReconciler()
	return 0, ""
}

func (rt *Router) recordStaticRollback(ctx context.Context, st staticRollbackStore, target store.DeployAttempt, stats deploy.StaticTreeStats) error {
	id, err := store.NewDeployAttemptID()
	if err != nil {
		return fmt.Errorf("mint attempt id: %w", err)
	}
	now := time.Now()
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: target.ServiceName, Image: target.Image, CommitSHA: target.CommitSHA,
		Source: store.DeployAttemptSourceManual, Status: store.DeployAttemptStatusRunning, StartedAt: now,
		Snapshot: target.Snapshot, Reason: "RollbackTo: " + target.ID,
	}); err != nil {
		return fmt.Errorf("save attempt: %w", err)
	}
	if err := st.SetDeployAttemptDigest(ctx, id, stats.Digest, store.DigestReasonStaticTree); err != nil {
		return fmt.Errorf("record digest: %w", err)
	}
	for _, e := range []store.DeployGateEvent{
		{Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, Reason: "StaticSitePublished", Message: "Rolled back the published files to deploy " + target.ID},
		{Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusPassed, Reason: "StaticSiteNoContainer", Message: "Static site: no container to watch"},
	} {
		e.DeployAttemptID, e.ServiceName, e.At = id, target.ServiceName, now
		if err := st.RecordGateEvent(ctx, e); err != nil {
			return fmt.Errorf("record gate event: %w", err)
		}
	}
	if err := st.SetDeployAttemptRollout(ctx, id, store.RolloutStateServing); err != nil {
		return fmt.Errorf("record rollout: %w", err)
	}
	if err := rt.deployAttempts.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		return fmt.Errorf("finish attempt: %w", err)
	}
	return nil
}
