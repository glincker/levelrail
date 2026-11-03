package api

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/imagelife"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
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
	t := time.NewTicker(imagePruneTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if rt.imagePruneDue(ctx, last, now) {
				last = now
				rt.pruneImagesOnce(ctx)
			}
		}
	}
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
	return !now.Before(sched.Next(last))
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
	removed, failed := 0, 0
	for _, e := range plan.Remove {
		if err := removeImageEntry(ctx, inv, e); err != nil {
			failed++
			continue
		}
		removed++
	}
	rt.logger.Info("api: scheduled image prune finished", slog.Int("removed", removed), slog.Int("failed", failed))
}
