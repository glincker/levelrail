package api

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/volumefiles"
)

// VolumeFileNode is what a node transport offers for browsing a named volume; the local node and agent transports both satisfy it.
type VolumeFileNode interface {
	VolumeList(ctx context.Context, volume, dir string, limit int) ([]volumefiles.Entry, bool, error)
	VolumeRead(ctx context.Context, volume, path string) (io.ReadCloser, error)
	VolumeWrite(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error
	VolumeRemove(ctx context.Context, volume, path string) error
}

type volumeFileListResource struct {
	Path      string              `json:"path"`
	Entries   []volumefiles.Entry `json:"entries"`
	Truncated bool                `json:"truncated"`
}

type volumeFileWriteResource struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type volumeFileRestoreRequest struct {
	BackupID string `json:"backup_id"`
	Path     string `json:"path"`
}

// volumeFileTarget resolves the app's node and the Docker volume, writing the error response on failure.
func (rt *Router) volumeFileTarget(w http.ResponseWriter, r *http.Request) (VolumeFileNode, string, bool) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "volume files are not configured on this control plane")
		return nil, "", false
	}
	name, vol := r.PathValue("name"), r.PathValue("volume")
	dockerVolume, ok := rt.loadServiceVolume(w, r, name, vol, "api: volume files: load service failed")
	if !ok {
		return nil, "", false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: volume files: load app failed", err, slog.String("name", name))
		return nil, "", false
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Error("api: volume files: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", svc.NodeID))
		writeError(w, http.StatusBadGateway, "the node holding this volume is not currently reachable")
		return nil, "", false
	}
	node, ok := nodeRuntime.(VolumeFileNode)
	if !ok {
		writeError(w, http.StatusNotImplemented, "the node holding this volume cannot browse volume files")
		return nil, "", false
	}
	return node, dockerVolume, true
}

// writeVolumeFileError maps a volumefiles error, which may have crossed the agent wire as text, to a status.
func (rt *Router) writeVolumeFileError(w http.ResponseWriter, msg string, err error, attrs ...slog.Attr) {
	switch e := volumefiles.Classify(err); {
	case errors.Is(e, volumefiles.ErrPathEscapes):
		writeError(w, http.StatusBadRequest, "path is outside the volume")
	case errors.Is(e, volumefiles.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such file in this volume")
	case errors.Is(e, volumefiles.ErrIsDirectory):
		writeError(w, http.StatusBadRequest, "path is a directory")
	case errors.Is(e, volumefiles.ErrNotRegular):
		writeError(w, http.StatusBadRequest, "path is not a regular file")
	case errors.Is(e, volumefiles.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d MB upload limit", volumefiles.MaxUploadBytes()>>20))
	default:
		rt.internalError(w, msg, err, attrs...)
	}
}

func (rt *Router) auditVolumeFile(r *http.Request, action, app, volume, filePath string, status int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: volume file audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: action, Path: "/api/v1/apps/" + app + "/volumes/" + volume + "/files#" + filePath, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: volume file audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}

// handleListVolumeFiles handles GET /api/v1/apps/{name}/volumes/{volume}/files.
func (rt *Router) handleListVolumeFiles(w http.ResponseWriter, r *http.Request) {
	node, dockerVolume, ok := rt.volumeFileTarget(w, r)
	if !ok {
		return
	}
	dir := r.URL.Query().Get("path")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &limit); err != nil || limit < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
	}
	entries, truncated, err := node.VolumeList(r.Context(), dockerVolume, dir, volumefiles.EffectiveLimit(limit))
	if err != nil {
		rt.writeVolumeFileError(w, "api: list volume files failed", err, slog.String("name", r.PathValue("name")))
		return
	}
	if entries == nil {
		entries = []volumefiles.Entry{}
	}
	writeJSON(w, http.StatusOK, volumeFileListResource{Path: dir, Entries: entries, Truncated: truncated})
}

// handleDownloadVolumeFile handles GET /api/v1/apps/{name}/volumes/{volume}/files/content.
func (rt *Router) handleDownloadVolumeFile(w http.ResponseWriter, r *http.Request) {
	node, dockerVolume, ok := rt.volumeFileTarget(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	rc, err := node.VolumeRead(r.Context(), dockerVolume, p)
	if err != nil {
		rt.writeVolumeFileError(w, "api: download volume file failed", err, slog.String("name", r.PathValue("name")))
		return
	}
	defer func() { _ = rc.Close() }()
	rt.auditVolumeFile(r, http.MethodGet, r.PathValue("name"), r.PathValue("volume"), p, http.StatusOK)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(p)}))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		rt.logger.Error("api: download volume file: stream failed", slog.String("error", err.Error()), slog.String("name", r.PathValue("name")))
	}
}

// handlePutVolumeFile handles PUT /api/v1/apps/{name}/volumes/{volume}/files/content with the raw file as the body.
func (rt *Router) handlePutVolumeFile(w http.ResponseWriter, r *http.Request) {
	node, dockerVolume, ok := rt.volumeFileTarget(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	limit := volumefiles.MaxUploadBytes()
	if r.ContentLength > limit {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d MB upload limit", limit>>20))
		return
	}
	cr := &countingReader{r: http.MaxBytesReader(w, r.Body, limit+1)}
	if err := node.VolumeWrite(r.Context(), dockerVolume, p, cr, limit); err != nil {
		rt.auditVolumeFile(r, http.MethodPut, r.PathValue("name"), r.PathValue("volume"), p, http.StatusInternalServerError)
		rt.writeVolumeFileError(w, "api: upload volume file failed", err, slog.String("name", r.PathValue("name")))
		return
	}
	rt.auditVolumeFile(r, http.MethodPut, r.PathValue("name"), r.PathValue("volume"), p, http.StatusOK)
	writeJSON(w, http.StatusOK, volumeFileWriteResource{Path: p, Size: cr.n})
}

// handleDeleteVolumeFile handles DELETE /api/v1/apps/{name}/volumes/{volume}/files/content: one file, never a directory.
func (rt *Router) handleDeleteVolumeFile(w http.ResponseWriter, r *http.Request) {
	node, dockerVolume, ok := rt.volumeFileTarget(w, r)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if err := node.VolumeRemove(r.Context(), dockerVolume, p); err != nil {
		rt.auditVolumeFile(r, http.MethodDelete, r.PathValue("name"), r.PathValue("volume"), p, http.StatusInternalServerError)
		rt.writeVolumeFileError(w, "api: delete volume file failed", err, slog.String("name", r.PathValue("name")))
		return
	}
	rt.auditVolumeFile(r, http.MethodDelete, r.PathValue("name"), r.PathValue("volume"), p, http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleRestoreVolumeFile handles POST /api/v1/apps/{name}/volumes/{volume}/files/restore: one file copied out of a succeeded backup.
func (rt *Router) handleRestoreVolumeFile(w http.ResponseWriter, r *http.Request) {
	if rt.backupDownloader == nil || rt.backupHistory == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}
	node, dockerVolume, ok := rt.volumeFileTarget(w, r)
	if !ok {
		return
	}
	var req volumeFileRestoreRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.BackupID == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "backup_id and path are required")
		return
	}
	name, vol := r.PathValue("name"), r.PathValue("volume")
	h, ok := rt.loadVolumeBackupHistory(w, r, name, vol, req.BackupID, "api: restore volume file: load backup failed")
	if !ok {
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be restored from", req.BackupID, h.Status))
		return
	}
	rel, err := volumefiles.Clean(req.Path)
	if err != nil || rel == "." {
		writeError(w, http.StatusBadRequest, "path is outside the volume")
		return
	}
	stream, err := rt.backupDownloader.Download(r.Context(), req.BackupID)
	if err != nil {
		rt.internalError(w, "api: restore volume file: download backup failed", err, slog.String("backup_id", req.BackupID))
		return
	}
	defer func() { _ = stream.Close() }()
	entry, err := tarEntry(stream, rel)
	if errors.Is(err, errTarEntryMissing) {
		writeError(w, http.StatusNotFound, "that file is not in this backup")
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore volume file: read backup failed", err, slog.String("backup_id", req.BackupID))
		return
	}
	limit := volumefiles.MaxUploadBytes()
	cr := &countingReader{r: entry}
	if err := node.VolumeWrite(r.Context(), dockerVolume, rel, cr, limit); err != nil {
		rt.auditVolumeFile(r, "RESTORE", name, vol, rel, http.StatusInternalServerError)
		rt.writeVolumeFileError(w, "api: restore volume file failed", err, slog.String("name", name))
		return
	}
	rt.auditVolumeFile(r, "RESTORE", name, vol, rel, http.StatusOK)
	writeJSON(w, http.StatusOK, volumeFileWriteResource{Path: rel, Size: cr.n})
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

var errTarEntryMissing = errors.New("api: file not found in backup archive")

// tarEntry returns a reader positioned on the regular file rel inside a (possibly gzipped) volume archive.
func tarEntry(r io.Reader, rel string) (io.Reader, error) {
	br := bufio.NewReader(r)
	if magic, err := br.Peek(2); err == nil && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, gerr := gzip.NewReader(br)
		if gerr != nil {
			return nil, gerr
		}
		r = gz
	} else {
		r = br
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errTarEntryMissing
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if strings.TrimPrefix(path.Clean("/"+hdr.Name), "/") == rel {
			return tr, nil
		}
	}
}
