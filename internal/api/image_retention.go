package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/imagelife"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions for per-app image retention.
const (
	AuditActionImageRetentionSet = "images.retention_set"
	AuditActionImageGCRemoved    = "images.gc_removed"
)

// Image keep sources, so the dashboard can say where an app's count comes from.
const (
	imageKeepSourceApp     = "app"
	imageKeepSourceDefault = "app_default"
	imageKeepSourceGlobal  = "global"
)

// imageKeepMu serializes read-modify-write of the per-app keep map.
var imageKeepMu sync.Mutex

type imageRetentionResource struct {
	App string `json:"app"`
	// Keep is the app's own count, 0 when it follows the defaults.
	Keep          int    `json:"keep"`
	EffectiveKeep int    `json:"effective_keep"`
	Source        string `json:"source"`
	GlobalKeep    int    `json:"global_keep"`
	MinKeep       int    `json:"min_keep"`
	// ProtectedEnvironment names the protected environment the app lives in, if any.
	ProtectedEnvironment string `json:"protected_environment,omitempty"`
}

type imageRetentionRequest struct {
	Keep *int   `json:"keep"`
	Node string `json:"node,omitempty"`
}

type imageRetentionCandidate struct {
	ID           string    `json:"id"`
	Tags         []string  `json:"tags"`
	SizeBytes    int64     `json:"size_bytes"`
	ReclaimBytes int64     `json:"reclaim_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

type imageRetentionKept struct {
	ID     string   `json:"id"`
	Tags   []string `json:"tags"`
	Reason string   `json:"reason"`
}

type imageRetentionPreview struct {
	App           string                    `json:"app"`
	Keep          int                       `json:"keep"`
	EffectiveKeep int                       `json:"effective_keep"`
	DryRun        bool                      `json:"dry_run"`
	Remove        []imageRetentionCandidate `json:"remove"`
	RemoveCount   int                       `json:"remove_count"`
	ReclaimBytes  int64                     `json:"reclaim_bytes"`
	Kept          []imageRetentionKept      `json:"kept"`
}

// errImageKeepUnreadable fails closed: no prune and no write runs on a map we cannot read.
var errImageKeepUnreadable = errors.New("the saved per-app image keep counts cannot be read; fix or reset the app.image_keep_overrides setting first")

func (rt *Router) appImageKeepOverrides(ctx context.Context) (map[string]int, error) {
	m, err := runtimesettings.ParseAppImageKeepOverrides(rt.imageSettings().String(ctx, runtimesettings.KeyAppImageKeepOverrides))
	if err != nil {
		rt.logger.Warn("api: per-app image keep unreadable", slog.String("error", err.Error()))
		return nil, errImageKeepUnreadable
	}
	return m, nil
}

// effectiveImageKeep resolves an app's keep count: its own, then the app default, then the global one.
// An app in a protected environment, or one whose environment cannot be read, never goes below the global count.
func (rt *Router) effectiveImageKeep(ctx context.Context, app string, overrides map[string]int) (n int, src string, protected bool) {
	global := rt.imageKeepPerApp()
	n, src = global, imageKeepSourceGlobal
	if v := overrides[app]; v > 0 {
		n, src = v, imageKeepSourceApp
	} else if v := rt.imageSettings().Int(ctx, runtimesettings.KeyAppImageKeep); v > 0 {
		n, src = v, imageKeepSourceDefault
	}
	_, found, err := rt.protectedEnvironmentOf(ctx, app)
	protected = found || err != nil
	if protected && n < global {
		n = global
	}
	return n, src, protected
}

// applyAppImageKeep sets each app's own keep; an explicit request count only ever raises it.
func (rt *Router) applyAppImageKeep(ctx context.Context, apps []imagelife.AppState, requested int) error {
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil {
		return err
	}
	for i := range apps {
		n, src, protected := rt.effectiveImageKeep(ctx, apps[i].Name, overrides)
		if src != imageKeepSourceGlobal || protected {
			apps[i].Keep = max(n, requested)
		}
	}
	return nil
}

// removalStillSafe rechecks an image right before removal, so a deploy that started after the plan keeps it.
func (rt *Router) removalStillSafe(ctx context.Context, e imagelife.Entry) bool {
	for app := range entryApps(e) {
		if svc, err := rt.apps.GetDesiredService(ctx, app); err == nil {
			if e.MatchesAttempt(imagelife.Attempt{Image: svc.Image}) || (svc.LocalImageID() != "" && svc.LocalImageID() == e.ID) {
				return false
			}
		} else if !errors.Is(err, store.ErrServiceNotFound) {
			return false
		}
		attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app)
		if err != nil {
			return false
		}
		for _, a := range attempts {
			if imagelife.IsPendingStatus(a.Status) && e.MatchesAttempt(imagelife.Attempt{Image: a.Image, Digest: a.ImageDigest}) {
				return false
			}
		}
	}
	return true
}

// forgetAppImageKeep drops a deleted app's keep count.
func (rt *Router) forgetAppImageKeep(ctx context.Context, app string) {
	imageKeepMu.Lock()
	defer imageKeepMu.Unlock()
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil || overrides[app] == 0 {
		return
	}
	delete(overrides, app)
	if err := rt.saveAppImageKeep(ctx, overrides, "app-delete"); err != nil {
		rt.logger.Warn("api: delete app: drop image keep failed", slog.String("error", err.Error()), slog.String("name", app))
	}
}

// pruneImageKeep drops counts of apps that no longer exist, so the map cannot grow without bound.
func (rt *Router) pruneImageKeep(ctx context.Context, overrides map[string]int) error {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}
	live := make(map[string]bool, len(services))
	for _, s := range services {
		live[s.Name] = true
	}
	for app := range overrides {
		if !live[app] {
			delete(overrides, app)
		}
	}
	return nil
}

// retentionSafeRemovals drops images more than one app references, so a shared base image is never pruned.
func retentionSafeRemovals(remove []imagelife.Entry) []imagelife.Entry {
	out := make([]imagelife.Entry, 0, len(remove))
	for _, e := range remove {
		if len(entryApps(e)) > 1 {
			continue
		}
		out = append(out, e)
	}
	return out
}

func entryApps(e imagelife.Entry) map[string]bool {
	apps := map[string]bool{}
	for _, u := range e.Apps {
		apps[u.App] = true
	}
	return apps
}

func (rt *Router) imageRetentionFor(ctx context.Context, name string) (imageRetentionResource, error) {
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil {
		return imageRetentionResource{}, err
	}
	eff, src, _ := rt.effectiveImageKeep(ctx, name, overrides)
	res := imageRetentionResource{
		App: name, Keep: overrides[name], EffectiveKeep: eff, Source: src,
		GlobalKeep: rt.imageKeepPerApp(), MinKeep: runtimesettings.AppImageKeepMin,
	}
	env, found, err := rt.protectedEnvironmentOf(ctx, name)
	if err != nil {
		return res, fmt.Errorf("protected environment of %s: %w", name, err)
	}
	if found {
		res.ProtectedEnvironment = env.Name
	}
	return res, nil
}

// appForRetention loads the app or writes 404.
func (rt *Router) appForRetention(w http.ResponseWriter, r *http.Request) (*store.DesiredService, bool) {
	svc, err := rt.apps.GetDesiredService(r.Context(), r.PathValue("name"))
	switch {
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	case err != nil:
		rt.internalError(w, "api: image retention: load app failed", err, slog.String("app", r.PathValue("name")))
		return nil, false
	}
	return svc, true
}

// handleGetImageRetention handles GET /api/v1/apps/{name}/image-retention.
func (rt *Router) handleGetImageRetention(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.appForRetention(w, r)
	if !ok {
		return
	}
	res, err := rt.imageRetentionFor(r.Context(), svc.Name)
	if errors.Is(err, errImageKeepUnreadable) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: image retention read failed", err, slog.String("app", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func decodeImageRetention(w http.ResponseWriter, r *http.Request, keepRequired bool) (imageRetentionRequest, bool) {
	var req imageRetentionRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return req, false
		}
	}
	if keepRequired && req.Keep == nil {
		writeError(w, http.StatusBadRequest, "keep is required")
		return req, false
	}
	if req.Keep != nil {
		if err := runtimesettings.CheckAppImageKeep(*req.Keep); err != nil {
			writeError(w, http.StatusBadRequest, "keep "+err.Error())
			return req, false
		}
	}
	return req, true
}

// protectedKeepRefusal refuses lowering a protected environment's app below the global count.
func protectedKeepRefusal(cur imageRetentionResource, keep int) string {
	if cur.ProtectedEnvironment == "" || keep == 0 || keep >= cur.GlobalKeep {
		return ""
	}
	return fmt.Sprintf("%s is in the protected environment %s, so it keeps at least the global %d images", cur.App, cur.ProtectedEnvironment, cur.GlobalKeep)
}

// handleSetImageRetention handles PUT /api/v1/apps/{name}/image-retention.
func (rt *Router) handleSetImageRetention(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.appForRetention(w, r)
	if !ok {
		return
	}
	req, ok := decodeImageRetention(w, r, true)
	if !ok {
		return
	}
	ctx := r.Context()
	cur, err := rt.imageRetentionFor(ctx, svc.Name)
	if errors.Is(err, errImageKeepUnreadable) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: image retention read failed", err, slog.String("app", svc.Name))
		return
	}
	if msg := protectedKeepRefusal(cur, *req.Keep); msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
	imageKeepMu.Lock()
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil {
		imageKeepMu.Unlock()
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	before := overrides[svc.Name]
	if *req.Keep == 0 {
		delete(overrides, svc.Name)
	} else {
		overrides[svc.Name] = *req.Keep
	}
	if err = rt.pruneImageKeep(ctx, overrides); err == nil {
		if cerr := runtimesettings.CheckAppImageKeepCount(len(overrides)); cerr != nil {
			imageKeepMu.Unlock()
			writeError(w, http.StatusConflict, cerr.Error())
			return
		}
		err = rt.saveAppImageKeep(ctx, overrides, rt.accessActor(r).name)
	}
	imageKeepMu.Unlock()
	if err != nil {
		rt.internalError(w, "api: image retention save failed", err, slog.String("app", svc.Name))
		return
	}
	rt.auditImageRetention(r, AuditActionImageRetentionSet, svc.Name, map[string]any{"before": before, "after": *req.Keep})
	res, err := rt.imageRetentionFor(ctx, svc.Name)
	if err != nil {
		rt.internalError(w, "api: image retention read failed", err, slog.String("app", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) saveAppImageKeep(ctx context.Context, overrides map[string]int, actor string) error {
	raw, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("encode per-app image keep: %w", err)
	}
	val, err := json.Marshal(string(raw))
	if err != nil {
		return fmt.Errorf("encode per-app image keep: %w", err)
	}
	if _, err := rt.imageSettings().Set(ctx, runtimesettings.KeyAppImageKeepOverrides, val, actor); err != nil {
		return fmt.Errorf("save per-app image keep: %w", err)
	}
	return nil
}

// handlePreviewImageRetention handles POST /api/v1/apps/{name}/image-retention/preview:
// a dry run of what the cleanup would remove from this app with the given keep. It never deletes.
func (rt *Router) handlePreviewImageRetention(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.appForRetention(w, r)
	if !ok {
		return
	}
	req, ok := decodeImageRetention(w, r, false)
	if !ok {
		return
	}
	node := req.Node
	if node == "" {
		node = svc.NodeID
	}
	inv, ok := rt.imageInventoryFor(w, node)
	if !ok {
		return
	}
	ctx := r.Context()
	records, err := inv.ListAllImages(ctx)
	if err != nil {
		rt.internalError(w, "api: image retention preview: list images failed", err, slog.String("app", svc.Name))
		return
	}
	apps, err := rt.loadImageApps(ctx, node)
	if err != nil {
		rt.internalError(w, "api: image retention preview: load apps failed", err, slog.String("app", svc.Name))
		return
	}
	if err := rt.applyAppImageKeep(ctx, apps, 0); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	out := imageRetentionPreview{App: svc.Name, DryRun: true, Remove: []imageRetentionCandidate{}, Kept: []imageRetentionKept{}}
	overrides, err := rt.appImageKeepOverrides(ctx)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	out.Keep = overrides[svc.Name]
	if req.Keep != nil {
		out.Keep = *req.Keep
		overrides[svc.Name] = *req.Keep
	}
	out.EffectiveKeep, _, _ = rt.effectiveImageKeep(ctx, svc.Name, overrides)
	for i := range apps {
		if apps[i].Name == svc.Name {
			apps[i].Keep = out.EffectiveKeep
		}
	}
	ret := rt.imageRetentionDefaults()
	entries := imagelife.Build(records, apps, ret.KeepPerApp, time.Duration(ret.MinAgeHrs)*time.Hour, time.Now())
	removing := map[string]bool{}
	for _, e := range retentionSafeRemovals(imagelife.PlanGC(entries).Remove) {
		if !entryApps(e)[svc.Name] {
			continue
		}
		removing[e.ID] = true
		out.Remove = append(out.Remove, imageRetentionCandidate{ID: e.ID, Tags: e.Tags, SizeBytes: e.SizeBytes, ReclaimBytes: imagelife.Reclaimable(e), CreatedAt: e.CreatedAt})
		out.ReclaimBytes += imagelife.Reclaimable(e)
	}
	for _, e := range entries {
		owners := entryApps(e)
		if !owners[svc.Name] || removing[e.ID] {
			continue
		}
		reason := e.ProtectReason
		if len(owners) > 1 {
			reason = "shared with another app"
		}
		if reason == "" {
			reason = "kept by the global cleanup rules"
		}
		out.Kept = append(out.Kept, imageRetentionKept{ID: e.ID, Tags: e.Tags, Reason: reason})
	}
	out.RemoveCount = len(out.Remove)
	writeJSON(w, http.StatusOK, out)
}

// auditImageRetention records a retention change or a removal with its detail.
func (rt *Router) auditImageRetention(r *http.Request, action, app string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: image retention audit id failed", slog.String("error", err.Error()))
		return
	}
	if app != "" {
		detail["app"] = app
	}
	body, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     action, Detail: string(body),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: image retention audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
