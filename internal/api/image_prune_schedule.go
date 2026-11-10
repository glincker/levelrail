package api

import (
	"context"
	"encoding/json"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/imagelife"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// imagePruneTick is how often the schedule setting is re-read, so a change applies within a minute.
const imagePruneTick = time.Minute

func (rt *Router) imageSettings() *runtimesettings.Service {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings
	}
	return runtimesettings.New(nil, func(k string) (string, bool) { return os.LookupEnv(k) })
}

// imageKeepPerApp reads the live keep count, falling back to env then the default.
func (rt *Router) imageKeepPerApp() int {
	return rt.imageSettings().Int(context.Background(), runtimesettings.KeyImagesKeepPerApp)
}

func (rt *Router) imagePruneSchedule(ctx context.Context) string {
	return rt.imageSettings().String(ctx, runtimesettings.KeyImagesPruneSchedule)
}

// RunImagePrune removes old images on the control plane node on the saved cron
// schedule. It does nothing while the schedule is off.
func (rt *Router) RunImagePrune(ctx context.Context) {
	last := time.Now()
	_ = footprint.Every(ctx, "image prune", imagePruneTick, rt.logger, func(ctx context.Context) error {
		now := time.Now()
		if rt.imagePruneDue(ctx, last, now) {
			last = now
			rt.pruneImagesOnce(ctx)
		}
		return nil
	})
}

func (rt *Router) imagePruneDue(ctx context.Context, last, now time.Time) bool {
	expr := rt.imagePruneSchedule(ctx)
	if expr == "" || expr == runtimesettings.ImagesPruneOff {
		return false
	}
	sched, err := cronexpr.Parse(expr)
	if err != nil {
		return false
	}
	next := sched.Next(last)
	return cronexpr.Due(next, now)
}

func (rt *Router) pruneImagesOnce(ctx context.Context) {
	if rt.imageInventory == nil {
		return
	}
	inv, err := rt.imageInventory("")
	if err != nil {
		rt.logger.Warn("api: scheduled image prune: node unreachable", slog.String("error", err.Error()))
		return
	}
	entries, err := rt.buildImageEntries(ctx, inv, "", rt.imageRetentionDefaults())
	if err != nil {
		rt.logger.Warn("api: scheduled image prune: plan failed", slog.String("error", err.Error()))
		return
	}
	plan := imagelife.PlanGC(entries)
	plan.Remove = retentionSafeRemovals(plan.Remove)
	removed, failed := []string{}, 0
	var refs, failures []string
	for _, e := range plan.Remove {
		if !rt.removalStillSafe(ctx, e) {
			continue
		}
		done, err := removeImageEntry(ctx, inv, e)
		refs = append(refs, done...)
		if err != nil {
			failed++
			failures = append(failures, e.ID)
			continue
		}
		removed = append(removed, e.ID)
	}
	rt.logger.Info("api: scheduled image prune finished", slog.Int("removed", len(removed)), slog.Int("failed", failed))
	if len(refs) > 0 || failed > 0 {
		rt.auditSystemImagePrune(ctx, "images.prune_scheduled", removed, refs, failures)
	}
}

// auditSystemImagePrune records which images a prune removed, under the system actor.
func (rt *Router) auditSystemImagePrune(ctx context.Context, action string, ids, refs, failed []string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	detail, _ := json.Marshal(map[string]any{"removed": ids, "refs": refs, "failed": failed})
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "image-prune", ActorName: "image prune",
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/images/gc", StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), Action: action, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save image prune audit failed", slog.String("error", err.Error()))
	}
}
