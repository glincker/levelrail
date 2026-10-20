package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupDownloader is the surface the backup download handler needs from
// internal/backup.DownloadRunner: resolve one already-succeeded backup
// attempt's live object into a stream, the read-only counterpart of
// BackupRunner and RestoreRunner. *backup.DownloadRunner satisfies this
// structurally; internal/api never imports internal/backup directly, the
// same boundary BackupRunner's own doc comment describes.
type BackupDownloader interface {
	Download(ctx context.Context, historyID string) (io.ReadCloser, error)
}

// handleDownloadBackup handles
// GET /api/v1/databases/{name}/backups/{historyId}/download: streams a
// previously succeeded backup attempt's own object straight through to
// the response body so an operator can save a local copy, inspect it, or
// move it outside Levelrail entirely.
//
// AbilityReadSensitive (see this route's own registration comment in
// router.go): this is a read, not a mutation, so it doesn't belong at
// AbilityWriteSensitive alongside handleTriggerBackup; but unlike
// handleListBackupHistory's metadata-only AbilityRead, the response body
// here is the actual dump, potentially the entire content of a production
// database, which is a materially more sensitive thing for a scoped
// bearer token to be able to read than a list of past attempt timestamps
// and sizes.
//
// Every check this handler can do before ever touching the bucket, it
// does first, the same "validate everything synchronously, before
// starting real work" discipline handleTriggerRestore already follows:
// a wrong database name, a backup ID that doesn't belong to it, or an
// attempt that never reached BackupStatusSucceeded are all reported here
// as a real HTTP status, before BackupDownloader.Download ever resolves a
// credential or opens a connection to the target bucket.
//
// The response is streamed via io.Copy directly from the object's own
// io.ReadCloser, never buffered whole into this process's memory first: a
// database dump can be large, and Downloader.Download's own doc comment
// (internal/backup/downloader.go) already establishes that same
// no-buffering contract for the identical stream on the restore path.
func (rt *Router) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupDownloader == nil {
		writeError(w, http.StatusNotImplemented, "backup downloads are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	historyID := r.PathValue("historyId")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: download backup: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h, err := rt.backupHistory.GetBackupHistory(r.Context(), historyID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: download backup: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.DatabaseName != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was taken from database %q, not %q", historyID, h.DatabaseName, name))
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be downloaded", historyID, h.Status))
		return
	}

	stream, err := rt.backupDownloader.Download(r.Context(), historyID)
	if err != nil {
		rt.logger.Error("api: download backup failed", slog.String("error", err.Error()), slog.String("backup_id", historyID), slog.String("database", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() {
		_ = stream.Close()
	}()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadFilename(h)))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, stream); err != nil {
		// Headers and a 200 status are already written by this point, so
		// there is no HTTP error left to report to the client: this can
		// only surface as a truncated download on their end, and the
		// honest thing left to do here is log it server-side, the same
		// "give up honestly rather than mask it" reasoning RunBackup's own
		// doc comment gives for a failure it similarly can't recover from.
		rt.logger.Error("api: download backup: stream failed", slog.String("error", err.Error()), slog.String("backup_id", historyID), slog.String("database", name))
	}
}

// downloadFilename derives a browser-facing filename for a backup
// attempt's download from its object key (Runner.RunBackup always writes
// "<db>/<db>-<timestamp>.dump", so path.Base of that is already a good,
// self-describing filename), falling back to database name plus started
// timestamp if the key ever has no usable base segment, so a handler
// change to the internal object-key format elsewhere in this codebase
// can't silently turn this into an empty or "." filename.
func downloadFilename(h store.BackupHistory) string {
	if base := path.Base(h.ObjectKey); base != "" && base != "." && base != "/" {
		return base
	}
	return fmt.Sprintf("%s-%s.dump", h.DatabaseName, strings.ReplaceAll(h.StartedAt, ":", ""))
}
