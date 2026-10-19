package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/archive"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxArchiveDays      = 3650
	archiveSweepDefault = time.Hour
)

// ArchiveSettingsStore persists the archive policy.
type ArchiveSettingsStore interface {
	GetArchiveSettings(ctx context.Context) (store.ArchiveSettings, error)
	UpdateArchiveSettings(ctx context.Context, s store.ArchiveSettings, now time.Time) error
	RecordArchiveRun(ctx context.Context, at time.Time, runErr string) error
}

// WithArchives enables the /api/v1/archives routes. Without it they return 501.
func WithArchives(m *archive.Manager, st ArchiveSettingsStore) Option {
	return func(rt *Router) { rt.archives, rt.archiveSettings = m, st }
}

func (rt *Router) archivesOrNotImplemented(w http.ResponseWriter) bool {
	if rt.archives == nil || rt.archiveSettings == nil {
		writeError(w, http.StatusNotImplemented, "archives are not configured")
		return false
	}
	return true
}

func (rt *Router) writeArchiveError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, archive.ErrInvalidID), errors.Is(err, archive.ErrNotFound):
		writeError(w, http.StatusNotFound, "archive not found")
	case errors.Is(err, archive.ErrNothingToArchive):
		writeError(w, http.StatusConflict, "no rows are old enough to archive")
	case errors.Is(err, archive.ErrChecksum):
		writeError(w, http.StatusUnprocessableEntity, "archive checksum mismatch: the bundle was modified or corrupted")
	default:
		rt.internalError(w, "api: archive "+action+" failed", err)
	}
}

type archiveListResponse struct {
	Bundles  []archive.Bundle      `json:"bundles"`
	Policy   store.ArchiveSettings `json:"policy"`
	HotRows  map[string]int        `json:"hot_rows"`
	SizeByte int64                 `json:"total_size_bytes"`
}

func (rt *Router) handleListArchives(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	list, err := rt.archives.List()
	if err != nil {
		rt.writeArchiveError(w, "list", err)
		return
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get archive policy failed", err)
		return
	}
	var total int64
	for _, b := range list {
		total += b.SizeBytes
	}
	writeJSON(w, http.StatusOK, archiveListResponse{Bundles: list, Policy: pol, HotRows: rt.archives.HotRows(r.Context()), SizeByte: total})
}

type createArchiveRequest struct {
	Kind          archive.Kind `json:"kind"`
	OlderThanDays int          `json:"older_than_days"`
}

func (rt *Router) handleCreateArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	var req createArchiveRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Kind == "" {
		res, err := rt.RunArchives(r.Context(), true)
		if err != nil {
			rt.writeArchiveError(w, "run", err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
		return
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get archive policy failed", err)
		return
	}
	days := req.OlderThanDays
	if days == 0 {
		days = map[archive.Kind]int{archive.KindAudit: pol.AuditHotDays, archive.KindDeploys: pol.DeployHotDays, archive.KindPipelines: pol.PipelineHotDays}[req.Kind]
	}
	if days < 1 || days > maxArchiveDays {
		writeError(w, http.StatusBadRequest, "kind must be audit, deploys or pipelines, and older_than_days between 1 and "+strconv.Itoa(maxArchiveDays))
		return
	}
	b, err := rt.archives.Create(r.Context(), req.Kind, time.Now().AddDate(0, 0, -days))
	if err != nil {
		rt.writeArchiveError(w, "create", err)
		return
	}
	writeJSON(w, http.StatusCreated, archive.RunResult{Created: []archive.Bundle{b}})
}

// RunArchives applies the archive policy once. A scheduled call does nothing
// while the policy is disabled; a manual one always runs.
func (rt *Router) RunArchives(ctx context.Context, manual bool) (archive.RunResult, error) {
	if rt.archives == nil || rt.archiveSettings == nil {
		return archive.RunResult{}, errors.New("archives are not configured")
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(ctx)
	if err != nil {
		return archive.RunResult{}, fmt.Errorf("get archive policy: %w", err)
	}
	if !manual && !pol.Enabled {
		return archive.RunResult{}, nil
	}
	res, runErr := rt.archives.Run(ctx, pol)
	msg := ""
	if runErr != nil {
		msg = runErr.Error()
	}
	if err := rt.archiveSettings.RecordArchiveRun(ctx, time.Now(), msg); err != nil {
		rt.logger.Warn("api: record archive run failed", slog.String("error", err.Error()))
	}
	return res, runErr
}

func (rt *Router) archiveWakeChan() chan struct{} {
	rt.archiveWakeOnce.Do(func() { rt.archiveWake = make(chan struct{}, 1) })
	return rt.archiveWake
}

func (rt *Router) archivePolicyEnabled(ctx context.Context) bool {
	if rt.archives == nil || rt.archiveSettings == nil {
		return false
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(ctx)
	return err != nil || pol.Enabled
}

// RunArchiveSweeper applies the archive policy on interval until ctx is done. While the policy is off it holds no timer and waits for the policy to be saved.
func (rt *Router) RunArchiveSweeper(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = archiveSweepDefault
	}
	loop := footprint.Track("archive sweep", interval)
	defer loop.Active()()
	wake := rt.archiveWakeChan()
	for {
		if !rt.archivePolicyEnabled(ctx) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wake:
				continue
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-wake:
			timer.Stop()
		case <-timer.C:
		}
		start := time.Now()
		res, err := rt.RunArchives(ctx, false)
		loop.Observe(start, err)
		if err != nil {
			rt.logger.Warn("api: archive sweep failed", slog.String("error", err.Error()))
		} else if len(res.Created) > 0 || res.Pruned > 0 {
			rt.logger.Info("api: archive sweep done", slog.Int("bundles", len(res.Created)), slog.Int("pruned", res.Pruned))
		}
	}
}

func (rt *Router) handleGetArchivePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get archive policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

func (rt *Router) handlePutArchivePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	var req store.ArchiveSettings
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	for name, d := range map[string]int{"audit_hot_days": req.AuditHotDays, "deploy_hot_days": req.DeployHotDays, "pipeline_hot_days": req.PipelineHotDays} {
		if d < 1 || d > maxArchiveDays {
			writeError(w, http.StatusBadRequest, name+" must be between 1 and "+strconv.Itoa(maxArchiveDays))
			return
		}
	}
	if req.KeepDays < 0 || req.KeepDays > maxArchiveDays {
		writeError(w, http.StatusBadRequest, "keep_days must be between 0 (forever) and "+strconv.Itoa(maxArchiveDays))
		return
	}
	if err := rt.archiveSettings.UpdateArchiveSettings(r.Context(), req, time.Now()); err != nil {
		rt.internalError(w, "api: update archive policy failed", err)
		return
	}
	pol, err := rt.archiveSettings.GetArchiveSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get archive policy failed", err)
		return
	}
	select {
	case rt.archiveWakeChan() <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, pol)
}

func (rt *Router) handleGetArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	b, err := rt.archives.Get(r.PathValue("id"))
	if err != nil {
		rt.writeArchiveError(w, "get", err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (rt *Router) handleBrowseArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil {
		limit = 50
	}
	rows, err := rt.archives.Browse(r.PathValue("id"), q.Get("table"), max(offset, 0), limit)
	if err != nil {
		rt.writeArchiveError(w, "browse", err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (rt *Router) handleDownloadArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	id := r.PathValue("id")
	b, err := rt.archives.Get(id)
	if err != nil {
		rt.writeArchiveError(w, "download", err)
		return
	}
	path, err := rt.archives.Path(id)
	if err != nil {
		rt.writeArchiveError(w, "download", err)
		return
	}
	f, err := os.Open(path) //nolint:gosec // path comes from a validated archive id
	if err != nil {
		rt.writeArchiveError(w, "download", err)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/zstd")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".ndjson.zst"))
	w.Header().Set("X-Content-SHA256", b.SHA256)
	http.ServeContent(w, r, id+".ndjson.zst", b.CreatedAt, f)
}

func (rt *Router) handleRestoreArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	res, err := rt.archives.Restore(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeArchiveError(w, "restore", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleVerifyArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	b, err := rt.archives.Verify(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, archive.ErrNotFound) || errors.Is(err, archive.ErrInvalidID) {
			rt.writeArchiveError(w, "verify", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"bundle": b, "ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bundle": b, "ok": true})
}

func (rt *Router) handleDeleteArchive(w http.ResponseWriter, r *http.Request) {
	if !rt.archivesOrNotImplemented(w) {
		return
	}
	if err := rt.archives.Delete(r.PathValue("id")); err != nil {
		rt.writeArchiveError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
