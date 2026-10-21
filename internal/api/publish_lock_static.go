package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// heldStaticStore is the held static build surface of the publish lock store.
type heldStaticStore interface {
	GetHeldStaticBuild(ctx context.Context, deployID string) (*store.StaticSite, error)
}

type staticSiteSaver interface {
	SaveStaticSite(ctx context.Context, site store.StaticSite) error
}

// deployPublishLocks lets automatic deploy triggers hold builds of a locked
// app; nil when the store cannot hold builds.
func (rt *Router) deployPublishLocks() deploy.PublishLocks {
	l, _ := rt.publishLocks.(deploy.PublishLocks)
	return l
}

// publishHeldStatic makes a held static build live and reports whether
// deployID named one. The site is written before the held row is retired, so
// a crash between them is repaired by retrying.
func (rt *Router) publishHeldStatic(w http.ResponseWriter, r *http.Request, svc store.DesiredService, deployID string) bool {
	hs, ok := rt.publishLocks.(heldStaticStore)
	if !ok {
		return false
	}
	site, err := hs.GetHeldStaticBuild(r.Context(), deployID)
	if errors.Is(err, store.ErrHeldBuildNotFound) || (err == nil && site.Name != svc.Name) {
		return false
	}
	saver, canSave := rt.staticSites.(staticSiteSaver)
	if err != nil || !canSave {
		rt.logger.Error("api: load held static build failed", slog.Any("error", err), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	if _, statErr := os.Stat(site.RootDir); statErr != nil {
		writeError(w, http.StatusGone, "the files of this held build are no longer on disk; redeploy from source instead")
		return true
	}
	if err := saver.SaveStaticSite(r.Context(), *site); err != nil {
		rt.logger.Error("api: publish held static build failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	rt.nudgeReconciler()
	if st, ok := rt.deployAttempts.(interface {
		SetDeployAttemptRollout(ctx context.Context, id, state string) error
	}); ok {
		if err := st.SetDeployAttemptRollout(r.Context(), deployID, store.RolloutStateServing); err != nil {
			rt.logger.Warn("api: mark published static build serving failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		}
	}
	if err := rt.publishLocks.MarkHeldBuildPublished(r.Context(), deployID, rt.eventActor(r), time.Now()); err != nil {
		rt.logger.Warn("api: retire published held build failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventPublishHeld, Title: "Published a held build", Detail: deployID})
	res := toAppResource(svc)
	writeJSON(w, http.StatusOK, deployTriggerResult{appResource: &res})
	return true
}
