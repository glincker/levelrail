package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/janitor"
)

const janitorHistoryDefault = 20

// NewJanitor builds the cleanup engine over this router's stores and attaches
// it, so the API, the disk Fix button and the reconcile loop share one engine.
func (rt *Router) NewJanitor(db janitor.Store, dk janitor.Docker) *janitor.Engine {
	batch, maxBatches := tokenCleanupBounds()
	eng := janitor.New(janitor.Deps{
		Store:             db,
		Docker:            dk,
		DiskFree:          rt.fixitDiskFree,
		DesiredContainers: rt.desiredContainerNames,
		OrphanVolumes: func(ctx context.Context) ([]docker.NamedVolume, error) {
			if rt.orphanedVolumes == nil {
				return nil, nil
			}
			return rt.computeOrphanedVolumes(ctx)
		},
		RemoveVolume: func(ctx context.Context, name string) error {
			if rt.orphanedVolumes == nil {
				return errors.New("volume management is not configured")
			}
			return rt.orphanedVolumes.RemoveVolume(ctx, name)
		},
		Logger:          rt.logger,
		TokenWindows:    rt.tokenWindows,
		TokenBatch:      batch,
		TokenMaxBatches: maxBatches,
		Audit:           rt.recordTokenCleanupAudit,
	})
	rt.janitor = eng
	rt.janitorStore = db
	return eng
}

type janitorStatusResource struct {
	Policy         janitor.Policy      `json:"policy"`
	FreePercent    float64             `json:"free_percent"`
	FreeBytes      int64               `json:"free_bytes"`
	TotalBytes     int64               `json:"total_bytes"`
	Pressure       bool                `json:"pressure"`
	LastRun        *janitor.Report     `json:"last_run,omitempty"`
	Quarantine     []janitorQuarantine `json:"quarantine"`
	TotalReclaimed int64               `json:"total_reclaimed_bytes"`
}

type janitorQuarantine struct {
	Name          string    `json:"name"`
	SizeBytes     int64     `json:"size_bytes"`
	FirstSeen     time.Time `json:"first_seen"`
	DeletableAt   time.Time `json:"deletable_at"`
	DeleteEnabled bool      `json:"delete_enabled"`
	Ready         bool      `json:"ready"`
}

func (rt *Router) janitorReady(w http.ResponseWriter) bool {
	if rt.janitor == nil || rt.janitorStore == nil {
		writeError(w, http.StatusNotImplemented, "cleanup is not configured on this control plane")
		return false
	}
	return true
}

// handleGetJanitor handles GET /api/v1/system/janitor.
func (rt *Router) handleGetJanitor(w http.ResponseWriter, r *http.Request) {
	if !rt.janitorReady(w) {
		return
	}
	ctx := r.Context()
	policy, err := janitor.LoadPolicy(ctx, rt.janitorStore)
	if err != nil {
		rt.internalError(w, "api: load janitor policy failed", err)
		return
	}
	res := janitorStatusResource{Policy: policy, FreePercent: -1, Quarantine: []janitorQuarantine{}}
	if free, total, derr := rt.fixitDiskFree(ctx); derr == nil && total > 0 {
		res.FreeBytes, res.TotalBytes = free, total
		res.FreePercent = float64(free) * 100 / float64(total)
		res.Pressure = res.FreePercent < policy.DiskFreeBudgetPercent
	}
	runs, err := rt.janitor.History(ctx, 50)
	if err != nil {
		rt.internalError(w, "api: janitor history failed", err)
		return
	}
	for i := range runs {
		if i == 0 {
			res.LastRun = &runs[0]
		}
		if !runs[i].DryRun {
			res.TotalReclaimed += runs[i].ReclaimedBytes
		}
	}
	held, err := rt.janitorStore.ListJanitorQuarantine(ctx, "volume")
	if err != nil {
		rt.internalError(w, "api: janitor quarantine failed", err)
		return
	}
	hold := time.Duration(policy.VolumeQuarantineDays) * 24 * time.Hour
	for _, q := range held {
		due := q.FirstSeen.Add(hold)
		res.Quarantine = append(res.Quarantine, janitorQuarantine{
			Name: q.Name, SizeBytes: q.SizeBytes, FirstSeen: q.FirstSeen, DeletableAt: due,
			DeleteEnabled: policy.DeleteOrphanVolumes, Ready: policy.DeleteOrphanVolumes && !time.Now().Before(due),
		})
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUpdateJanitorPolicy handles PUT /api/v1/system/janitor/policy.
func (rt *Router) handleUpdateJanitorPolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.janitorReady(w) {
		return
	}
	policy, err := janitor.LoadPolicy(r.Context(), rt.janitorStore)
	if err != nil {
		rt.internalError(w, "api: load janitor policy failed", err)
		return
	}
	if err := decodeJSONBody(r, &policy); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := janitor.SavePolicy(r.Context(), rt.janitorStore, policy, actor, time.Now()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

// handleRunJanitor handles POST /api/v1/system/janitor/run. dry_run=true
// previews without changing anything. force=false waits for disk pressure.
func (rt *Router) handleRunJanitor(w http.ResponseWriter, r *http.Request) {
	if !rt.janitorReady(w) {
		return
	}
	q := r.URL.Query()
	opts := janitor.Options{Trigger: janitor.TriggerManual, DryRun: q.Get("dry_run") == "true", Force: q.Get("force") != "false"}
	rep, err := rt.janitor.Run(r.Context(), opts)
	if errors.Is(err, janitor.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: janitor run failed", err, slog.Bool("dry_run", opts.DryRun))
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleListJanitorRuns handles GET /api/v1/system/janitor/runs.
func (rt *Router) handleListJanitorRuns(w http.ResponseWriter, r *http.Request) {
	if !rt.janitorReady(w) {
		return
	}
	limit := janitorHistoryDefault
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	runs, err := rt.janitor.History(r.Context(), limit)
	if err != nil {
		rt.internalError(w, "api: janitor history failed", err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

// janitorDiskFix runs a forced, real pass for the disk Fix button and returns the bytes freed.
func (rt *Router) janitorDiskFix(ctx context.Context) (int64, error) {
	rep, err := rt.janitor.Run(ctx, janitor.Options{Trigger: janitor.TriggerDiskFix, Force: true})
	if err != nil {
		return 0, fmt.Errorf("cleanup: %w", err)
	}
	if rep.Error != "" {
		return rep.ReclaimedBytes, errors.New(rep.Error)
	}
	return rep.ReclaimedBytes, nil
}
