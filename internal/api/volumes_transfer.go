package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit events for moving a volume's files out of or into Levelrail.
const (
	AuditActionVolumeExport = "volume.export"
	AuditActionVolumeImport = "volume.import"
)

type volumeImportResource struct {
	Entries int64 `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

func (rt *Router) transferLimits(ctx context.Context) (maxBytes int64, timeout time.Duration) {
	maxBytes = int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyVolumesTransferMaxBytes))
	timeout = rt.runtimeSettings.Duration(ctx, runtimesettings.KeyVolumesTransferTimeout)
	if timeout < time.Minute {
		timeout = time.Minute
	}
	return maxBytes, timeout
}

// transferTarget resolves the app's volume and node runtime and refuses volumes holding protected files.
func (rt *Router) transferTarget(w http.ResponseWriter, r *http.Request) (store.DesiredService, string, backup.TransferRuntime, bool) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "volume transfers are not configured on this control plane")
		return store.DesiredService{}, "", nil, false
	}
	name, vol := r.PathValue("name"), r.PathValue("volume")
	dockerVolume, ok := rt.loadServiceVolume(w, r, name, vol, "api: volume transfer: load service failed")
	if !ok {
		return store.DesiredService{}, "", nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: volume transfer: load app failed", err, slog.String("name", name))
		return store.DesiredService{}, "", nil, false
	}
	if msg, err := rt.volumeOwnerProblem(r.Context(), *svc, dockerVolume); err != nil {
		rt.internalError(w, "api: volume transfer: owner check failed", err, slog.String("name", name))
		return store.DesiredService{}, "", nil, false
	} else if msg != "" {
		writeError(w, http.StatusConflict, msg)
		return store.DesiredService{}, "", nil, false
	}
	protected, err := rt.volumeProtectedPaths(r.Context(), dockerVolume)
	if err != nil {
		rt.internalError(w, "api: volume transfer: protected path check failed", err, slog.String("name", name))
		return store.DesiredService{}, "", nil, false
	}
	if protected != nil {
		writeError(w, http.StatusForbidden, msgVolumePathProtected)
		return store.DesiredService{}, "", nil, false
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Error("api: volume transfer: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", svc.NodeID))
		writeError(w, http.StatusBadGateway, "the node holding this volume is not currently reachable")
		return store.DesiredService{}, "", nil, false
	}
	return *svc, dockerVolume, nodeRuntime, true
}

// volumeOwnerProblem refuses a docker volume that is not this app's own: its name must
// carry the app's prefix and no other app may hold a record for the same volume.
func (rt *Router) volumeOwnerProblem(ctx context.Context, svc store.DesiredService, dockerVolume string) (string, error) {
	if !strings.HasPrefix(dockerVolume, store.ServiceVolumeName(svc.Name, "")) {
		return "this volume does not belong to the app", nil
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return "", err
	}
	if other, clash := store.ServiceVolumeOwnerConflict(all, svc.Name, dockerVolume); clash {
		return fmt.Sprintf("this volume name is also claimed by app %q: rename one of the volumes before moving data", other), nil
	}
	return "", nil
}

// handleExportVolume handles GET /api/v1/apps/{name}/volumes/{volume}/export: the volume as a tar.gz,
// read through a read-only helper container.
func (rt *Router) handleExportVolume(w http.ResponseWriter, r *http.Request) {
	_, dockerVolume, nodeRuntime, ok := rt.transferTarget(w, r)
	if !ok {
		return
	}
	app, vol := r.PathValue("name"), r.PathValue("volume")
	maxBytes, timeout := rt.transferLimits(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	stream, err := (&backup.VolumeTransfer{Runtime: nodeRuntime}).Export(ctx, dockerVolume, maxBytes)
	if err != nil {
		rt.logger.Error("api: export volume failed", slog.String("error", err.Error()), slog.String("name", app))
		rt.auditVolumeTransfer(r, AuditActionVolumeExport, app, vol, backup.TransferStats{}, http.StatusBadGateway)
		writeError(w, http.StatusBadGateway, "the node could not start the export")
		return
	}
	status := http.StatusOK
	defer func() {
		_ = stream.Close()
		rt.auditVolumeTransfer(r, AuditActionVolumeExport, app, vol, backup.TransferStats{Bytes: stream.Bytes()}, status)
	}()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", app+"-"+vol+".tar.gz"))
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, stream)
	if err == nil {
		err = ctx.Err() // a timeout can end the helper's stream without a read error
	}
	if err != nil {
		status = http.StatusInternalServerError
		rt.logger.Error("api: export volume: stream failed", slog.String("error", err.Error()), slog.String("name", app))
		// Headers are gone, so only an aborted response tells the client the archive is cut short.
		panic(http.ErrAbortHandler)
	}
}

// handleImportVolume handles POST /api/v1/apps/{name}/volumes/{volume}/import with the tar.gz as the body.
// It only fills an empty volume of a stopped app unless overwrite is asked for and allowed.
func (rt *Router) handleImportVolume(w http.ResponseWriter, r *http.Request) {
	svc, dockerVolume, nodeRuntime, ok := rt.transferTarget(w, r)
	if !ok {
		return
	}
	app, vol := r.PathValue("name"), r.PathValue("volume")
	fail := func(status int, msg string) {
		rt.auditVolumeTransfer(r, AuditActionVolumeImport, app, vol, backup.TransferStats{}, status)
		writeError(w, status, msg)
	}
	if !svc.Suspended {
		fail(http.StatusConflict, "stop the app before importing into its volume")
		return
	}
	overwrite := r.URL.Query().Get("force") == "true"
	if overwrite && !rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyVolumesImportOverwrite) {
		fail(http.StatusForbidden, "overwriting a volume that holds files is turned off: enable the volumes.import_overwrite setting first")
		return
	}
	maxBytes, timeout := rt.transferLimits(r.Context())
	if r.ContentLength > maxBytes {
		fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("archive exceeds the %d MB transfer limit", maxBytes>>20))
		return
	}
	spoolDir := rt.transferSpoolDir(r.Context())
	release, err := rt.volumeTransfers.beginImport(app, dockerVolume, rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyVolumesTransferMaxConcurrent), spoolDir)
	if err != nil {
		status, msg := importErrorStatus(err)
		fail(status, msg)
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	body := http.MaxBytesReader(w, r.Body, maxBytes)
	stats, err := (&backup.VolumeTransfer{Runtime: nodeRuntime}).Import(ctx, dockerVolume, body, backup.ImportOptions{
		MaxBytes: maxBytes, Overwrite: overwrite, SpoolDir: spoolDir,
		BeforeMount: func(ctx context.Context) error { return rt.recheckImportTarget(ctx, app, vol, dockerVolume) },
	})
	if err != nil {
		status, msg := importErrorStatus(err)
		if status == http.StatusInternalServerError {
			rt.logger.Error("api: import volume failed", slog.String("error", err.Error()), slog.String("name", app))
		}
		fail(status, msg)
		return
	}
	rt.auditVolumeTransfer(r, AuditActionVolumeImport, app, vol, stats, http.StatusOK)
	writeJSON(w, http.StatusOK, volumeImportResource{Entries: stats.Entries, Bytes: stats.Bytes})
}

// transferSpoolDir is where uploads are staged: a folder under the data dir, not the shared temp dir.
func (rt *Router) transferSpoolDir(ctx context.Context) string {
	name := rt.runtimeSettings.String(ctx, runtimesettings.KeyVolumesTransferSpoolDir)
	if name == "" || strings.ContainsAny(name, "/\\") || name == ".." {
		name = runtimesettings.DefaultVolumesTransferSpoolDir
	}
	if rt.dataDir == "" {
		return filepath.Join(os.TempDir(), "levelrail-"+name)
	}
	return filepath.Join(rt.dataDir, name)
}

// recheckImportTarget runs after the upload and just before the volume is mounted: the owning
// app must still exist, still own the volume, and still be stopped.
func (rt *Router) recheckImportTarget(ctx context.Context, app, vol, dockerVolume string) error {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return err
	}
	if got, ok := store.ServiceVolumeDockerName(*svc, vol); !ok || got != dockerVolume {
		return errImportAppNotStopped
	}
	if !svc.Suspended {
		return errImportAppNotStopped
	}
	if msg, err := rt.volumeOwnerProblem(ctx, *svc, dockerVolume); err != nil || msg != "" {
		if err == nil {
			err = errImportAppNotStopped
		}
		return err
	}
	return nil
}

func importErrorStatus(err error) (int, string) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, errImportVolumeBusy):
		return http.StatusConflict, "another import into this volume is already running"
	case errors.Is(err, errImportCapacity):
		return http.StatusTooManyRequests, "too many volume imports are running: try again when one finishes"
	case errors.Is(err, errImportAppNotStopped):
		return http.StatusConflict, "stop the app before importing into its volume, and keep it stopped until the import ends"
	case errors.Is(err, backup.ErrVolumeNotEmpty):
		return http.StatusConflict, "the volume already holds files: import into an empty volume, or pass force once volumes.import_overwrite is enabled"
	case errors.Is(err, backup.ErrTransferTooLarge), errors.As(err, &tooBig):
		return http.StatusRequestEntityTooLarge, "the archive exceeds the transfer size limit"
	case errors.Is(err, backup.ErrUnsafeArchive):
		return http.StatusBadRequest, strings.TrimPrefix(err.Error(), "backup: ")
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "the import ran past the transfer time limit"
	case strings.Contains(err.Error(), "not a gzip archive"), strings.Contains(err.Error(), "read archive"):
		return http.StatusBadRequest, "the body is not a valid tar.gz archive"
	}
	return http.StatusInternalServerError, "the import failed"
}

// auditVolumeTransfer records names and byte counts only, never file contents.
func (rt *Router) auditVolumeTransfer(r *http.Request, action, app, volume string, st backup.TransferStats, status int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: volume transfer audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWriteSensitive, Method: r.Method, Path: "/api/v1/apps/" + app + "/volumes/" + volume + "/" + strings.TrimPrefix(action, "volume."), StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     action, Detail: fmt.Sprintf(`{"app":%q,"volume":%q,"entries":%d,"bytes":%d}`, app, volume, st.Entries, st.Bytes),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: volume transfer audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
