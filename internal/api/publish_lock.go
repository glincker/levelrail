package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	publishStateHeld    = "held"
	maxPublishLockNote  = 280
	publishLockNotFound = "app not found"
)

// PublishLockStore is the store surface for per-app publish locks and the
// builds they hold. *store.DB satisfies this structurally.
type PublishLockStore interface {
	GetPublishLock(ctx context.Context, name string) (*store.PublishLock, error)
	SetPublishLock(ctx context.Context, l store.PublishLock) (store.PublishLock, error)
	ClearPublishLock(ctx context.Context, name string) (bool, error)
	ListHeldBuildIDs(ctx context.Context, name string) (map[string]bool, error)
	GetHeldBuild(ctx context.Context, deployID string) (*store.DesiredService, error)
	MarkHeldBuildPublished(ctx context.Context, deployID, by string, at time.Time) error
}

// WithPublishLocks enables per-app publish locks: pushes and hooks keep
// building but are held until an operator publishes them.
func WithPublishLocks(s PublishLockStore) Option {
	return func(rt *Router) { rt.publishLocks = s }
}

type publishLockResource struct {
	Locked bool `json:"locked"`
	*store.PublishLock
	HeldBuilds []string `json:"held_builds"`
}

type putPublishLockRequest struct {
	Reason string `json:"reason"`
}

// heldBuildIDs returns the app's unpublished held build ids; a failed lookup
// reads as none so the history list still renders.
func (rt *Router) heldBuildIDs(ctx context.Context, name string) map[string]bool {
	if rt.publishLocks == nil {
		return nil
	}
	ids, err := rt.publishLocks.ListHeldBuildIDs(ctx, name)
	if err != nil {
		rt.logger.Warn("api: list held builds failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	return ids
}

func (rt *Router) publishLockApp(w http.ResponseWriter, r *http.Request) (*store.DesiredService, bool) {
	if rt.publishLocks == nil {
		writeError(w, http.StatusNotImplemented, "publish locks are not configured on this control plane")
		return nil, false
	}
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, publishLockNotFound)
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: publish lock: load app failed", err, slog.String("name", name))
		return nil, false
	}
	return svc, true
}

func (rt *Router) writePublishLock(w http.ResponseWriter, r *http.Request, name string) {
	lock, err := rt.publishLocks.GetPublishLock(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get publish lock failed", err, slog.String("name", name))
		return
	}
	held := []string{}
	for id := range rt.heldBuildIDs(r.Context(), name) {
		held = append(held, id)
	}
	writeJSON(w, http.StatusOK, publishLockResource{Locked: lock != nil, PublishLock: lock, HeldBuilds: held})
}

// handleGetPublishLock handles GET /api/v1/apps/{name}/publish-lock.
func (rt *Router) handleGetPublishLock(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.publishLockApp(w, r); ok {
		rt.writePublishLock(w, r, r.PathValue("name"))
	}
}

// lockApp stores the lock as desired state; locking twice keeps the first lock.
func (rt *Router) lockApp(r *http.Request, svc store.DesiredService, reason string) error {
	if _, err := rt.publishLocks.SetPublishLock(r.Context(), store.PublishLock{
		ServiceName: svc.Name, Reason: reason, LockedBy: rt.eventActor(r), LockedAt: time.Now(), PinnedImage: svc.Image,
	}); err != nil {
		return err
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventPublishLock, Title: "Publishing locked", Detail: reason})
	return nil
}

// handlePutPublishLock handles PUT /api/v1/apps/{name}/publish-lock: new
// builds still run but are held, the live release stays pinned.
func (rt *Router) handlePutPublishLock(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.publishLockApp(w, r)
	if !ok {
		return
	}
	var req putPublishLockRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > maxPublishLockNote {
		writeError(w, http.StatusBadRequest, "reason is too long")
		return
	}
	if err := rt.lockApp(r, *svc, reason); err != nil {
		rt.internalError(w, "api: set publish lock failed", err, slog.String("name", svc.Name))
		return
	}
	rt.writePublishLock(w, r, svc.Name)
}

// handleDeletePublishLock handles DELETE /api/v1/apps/{name}/publish-lock.
// Already held builds stay publishable; only later builds go live on their own.
func (rt *Router) handleDeletePublishLock(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.publishLockApp(w, r)
	if !ok {
		return
	}
	cleared, err := rt.publishLocks.ClearPublishLock(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: clear publish lock failed", err, slog.String("name", svc.Name))
		return
	}
	if cleared {
		rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventPublishUnlock, Title: "Publishing unlocked"})
	}
	rt.writePublishLock(w, r, svc.Name)
}

// handlePublishHeldBuild handles POST /api/v1/apps/{name}/deploys/{deployId}/publish:
// makes a held build live. Writing desired state comes first and retiring
// the held row second, so a crash between them is repaired by retrying.
func (rt *Router) handlePublishHeldBuild(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.publishLockApp(w, r)
	if !ok {
		return
	}
	deployID := r.PathValue("deployId")
	if rt.publishHeldStatic(w, r, *svc, deployID) {
		return
	}
	held, err := rt.publishLocks.GetHeldBuild(r.Context(), deployID)
	if errors.Is(err, store.ErrHeldBuildNotFound) || (err == nil && held.Name != svc.Name) {
		writeError(w, http.StatusNotFound, "no held build with that id")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load held build failed", err, slog.String("deploy_id", deployID))
		return
	}
	held.NodeID = svc.NodeID
	appID := svc.AppID
	if appID == "" {
		appID = svc.Name
	}
	// Ports were admitted at hold time; another app may have taken one since.
	portsRelease, portsOK := rt.admitPublishedPorts(w, r, appID, []store.DesiredService{*svc}, []store.DesiredService{*held})
	if !portsOK {
		return
	}
	err = rt.apps.SaveDesiredService(r.Context(), *held)
	portsRelease()
	if err != nil {
		rt.internalError(w, "api: publish held build failed", err, slog.String("deploy_id", deployID))
		return
	}
	rt.nudgeReconciler()
	if err := rt.publishLocks.MarkHeldBuildPublished(r.Context(), deployID, rt.eventActor(r), time.Now()); err != nil {
		rt.logger.Warn("api: retire published held build failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventPublishHeld, Title: "Published a held build", Detail: deployID})
	res := toAppResource(*held)
	writeJSON(w, http.StatusOK, deployTriggerResult{appResource: &res})
}

// handleRollbackAndLock handles POST /api/v1/apps/{name}/deploys/{deployId}/rollback-lock:
// locks first so a failure midway leaves the app pinned, then rolls back.
func (rt *Router) handleRollbackAndLock(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.publishLockApp(w, r)
	if !ok {
		return
	}
	if err := rt.lockApp(r, *svc, "Rolled back and locked"); err != nil {
		rt.internalError(w, "api: rollback and lock failed", err, slog.String("name", svc.Name))
		return
	}
	rt.handleRollbackToDeploy(w, r)
}
