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

	"github.com/GLINCKER/levelrail/internal/sourceupload"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	uploadURLPrefix = "upload://"

	envUploadMaxBytes     = "APP_UPLOAD_MAX_BYTES"
	envUploadMaxFiles     = "APP_UPLOAD_MAX_FILES"
	envUploadMaxExtracted = "APP_UPLOAD_MAX_EXTRACTED_BYTES"
	envUploadRetention    = "APP_UPLOAD_RETENTION"

	defaultUploadMaxBytes     int64 = 512 << 20
	defaultUploadMaxFiles     int64 = 50000
	defaultUploadMaxExtracted int64 = 2 << 30
	defaultUploadRetention          = 7 * 24 * time.Hour
)

// sourceUploadResource is the JSON shape of one stored upload.
type sourceUploadResource struct {
	Hash           string `json:"hash"`
	Size           int64  `json:"size"`
	FileCount      int    `json:"file_count"`
	ExtractedBytes int64  `json:"extracted_bytes"`
	CreatedAt      string `json:"created_at"`
	Deduped        bool   `json:"deduped,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	// HasIndex and SiteRoot are set only for zip uploads: whether index.html is at the site root, and the wrapper folder removed.
	HasIndex *bool  `json:"has_index,omitempty"`
	SiteRoot string `json:"site_root,omitempty"`
}

type sourceUploadListResponse struct {
	Items      []sourceUploadResource `json:"items"`
	UsageBytes int64                  `json:"usage_bytes"`
}

// uploadStore returns the archive store under the data directory, or nil when none is configured.
func (rt *Router) uploadStore() *sourceupload.Store {
	if rt.dataDir == "" {
		return nil
	}
	return sourceupload.NewStore(filepath.Join(rt.dataDir, "uploads"), uploadLimits())
}

func uploadLimits() sourceupload.Limits {
	return sourceupload.Limits{
		MaxFiles:     int(envInt64Positive(envUploadMaxFiles, defaultUploadMaxFiles)),
		MaxBytes:     envInt64Positive(envUploadMaxExtracted, defaultUploadMaxExtracted),
		MaxFileBytes: envInt64Positive(envUploadMaxExtracted, defaultUploadMaxExtracted),
	}
}

func uploadRetention() time.Duration { return envDuration(envUploadRetention, defaultUploadRetention) }

func uploadResource(m sourceupload.Meta, deduped bool) sourceUploadResource {
	return sourceUploadResource{
		Hash: m.Hash, Size: m.Size, FileCount: m.Files, ExtractedBytes: m.Expanded, Deduped: deduped,
		CreatedAt: m.CreatedAt.Format(time.RFC3339), ExpiresAt: m.CreatedAt.Add(uploadRetention()).Format(time.RFC3339),
	}
}

// CleanupUploads removes stored uploads older than the retention window.
func (rt *Router) CleanupUploads(ctx context.Context) {
	st := rt.uploadStore()
	if st == nil {
		return
	}
	n, err := st.Cleanup(uploadRetention())
	if err != nil {
		rt.logger.Warn("api: upload cleanup failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		rt.logger.InfoContext(ctx, "api: expired source uploads removed", slog.Int("removed", n))
	}
}

// RunUploadCleanup expires stored uploads hourly until ctx ends.
func (rt *Router) RunUploadCleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		rt.CleanupUploads(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// handleUploadSource stores a gzip tar of the caller's directory for a later build.
func (rt *Router) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	st := rt.uploadStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "source upload is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: upload source: load app failed", err, slog.String("name", name))
		return
	}
	limit := envInt64Positive(envUploadMaxBytes, defaultUploadMaxBytes)
	if r.ContentLength > limit {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("upload is larger than the %d byte limit (%s)", limit, envUploadMaxBytes))
		return
	}
	rt.CleanupUploads(r.Context())
	var body io.Reader = r.Body
	var zipRes *sourceupload.ZipResult
	if isZipUpload(r) {
		zb, zr, cleanup, ok := rt.convertZipBody(w, r, limit)
		if !ok {
			return
		}
		defer cleanup()
		body, zipRes = zb, &zr
	}
	meta, deduped, err := st.Put(name, body, limit)
	switch {
	case errors.Is(err, sourceupload.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "upload is too large: "+err.Error())
		return
	case errors.Is(err, sourceupload.ErrUnsafeArchive):
		rt.logger.Warn("api: upload source rejected", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "archive rejected: "+err.Error())
		return
	case errors.Is(err, sourceupload.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "invalid app name")
		return
	case err != nil:
		rt.logger.Error("api: upload source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadRequest, "could not read the archive: it must be a gzip compressed tar")
		return
	}
	rt.logger.Info("api: source uploaded", slog.String("name", name), slog.String("hash", meta.Hash), slog.Int("files", meta.Files), slog.Int64("bytes", meta.Size), slog.Bool("deduped", deduped))
	status := http.StatusCreated
	if deduped {
		status = http.StatusOK
	}
	res := uploadResource(meta, deduped)
	if zipRes != nil {
		res.HasIndex, res.SiteRoot = &zipRes.HasIndex, zipRes.StrippedDir
	}
	writeJSON(w, status, res)
}

// handleListUploads lists an app's retained uploads and the total archive disk use.
func (rt *Router) handleListUploads(w http.ResponseWriter, r *http.Request) {
	st := rt.uploadStore()
	if st == nil {
		writeJSON(w, http.StatusOK, sourceUploadListResponse{Items: []sourceUploadResource{}})
		return
	}
	metas, err := st.List(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app name")
		return
	}
	usage, _ := st.Usage()
	out := sourceUploadListResponse{Items: make([]sourceUploadResource, 0, len(metas)), UsageBytes: usage}
	for _, m := range metas {
		out.Items = append(out.Items, uploadResource(m, false))
	}
	writeJSON(w, http.StatusOK, out)
}

// prepareUploadBuild turns req.UploadHash into the internal source reference.
// It returns an HTTP status and message when the request cannot proceed.
func (rt *Router) prepareUploadBuild(name string, req *triggerBuildRequest, buildType string) (sourceupload.Meta, int, string) {
	if buildType != spec.BuildDockerfile && buildType != spec.BuildRailpack && buildType != spec.BuildStatic {
		return sourceupload.Meta{}, http.StatusBadRequest, fmt.Sprintf("upload_hash needs build.type %q, %q or %q, not %q", spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic, buildType)
	}
	if req.RepoURL != "" || req.FromCache {
		return sourceupload.Meta{}, http.StatusBadRequest, "upload_hash cannot be combined with repo_url or from_cache"
	}
	st := rt.uploadStore()
	if st == nil {
		return sourceupload.Meta{}, http.StatusNotImplemented, "source upload is not configured on this control plane"
	}
	meta, err := st.Get(name, req.UploadHash)
	switch {
	case errors.Is(err, sourceupload.ErrNotFound):
		return meta, http.StatusNotFound, "upload not found or expired, send the directory again"
	case err != nil:
		return meta, http.StatusBadRequest, "invalid upload_hash"
	}
	req.RepoURL = uploadURLPrefix + meta.Hash
	req.Ref = meta.Hash[:40]
	return meta, 0, ""
}

func uploadMessage(m sourceupload.Meta) string {
	return fmt.Sprintf("Uploaded from CLI, %d files, %s", m.Files, m.Hash[:12])
}

// fetchUpload extracts a stored upload into a temp directory the way fetch checks out a git ref.
func (rt *Router) fetchUpload(app, repoURL string) (dir, commit string, cleanup func(), err error) {
	st := rt.uploadStore()
	if st == nil {
		return "", "", nil, errors.New("source upload is not configured")
	}
	hash := strings.TrimPrefix(repoURL, uploadURLPrefix)
	tmp, err := os.MkdirTemp("", "upload-src-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("create build directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if _, err := st.Extract(app, hash, tmp); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("extract upload %s: %w", hash, err)
	}
	return tmp, hash[:40], cleanup, nil
}
