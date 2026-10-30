package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appFilesUpdater is the one write this feature adds to the app store; kept
// off the shared apps interface so existing test fakes need no stub.
type appFilesUpdater interface {
	UpdateServiceFiles(ctx context.Context, name string, files []store.ServiceFile) error
}

// appFileResource is one file on the wire. A secret file never carries
// content: only its path, mode and the redacted flag.
type appFileResource struct {
	Path      string `json:"path"`
	Mode      string `json:"mode"`
	Secret    bool   `json:"secret"`
	Redacted  bool   `json:"redacted"`
	SizeBytes int    `json:"size_bytes,omitempty"`
	Content   string `json:"content,omitempty"`
}

type appFilesResource struct {
	AppName string            `json:"app_name"`
	Files   []appFileResource `json:"files"`
}

type setAppFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
	Secret  bool   `json:"secret"`
}

func appFilesResourceFrom(name string, files []store.ServiceFile) appFilesResource {
	out := appFilesResource{AppName: name, Files: make([]appFileResource, 0, len(files))}
	for _, f := range files {
		mode := f.Mode
		if mode == "" {
			mode = spec.DefaultFileMode
		}
		r := appFileResource{Path: f.Path, Mode: mode, Secret: f.Secret, Redacted: f.Secret}
		if !f.Secret {
			r.Content, r.SizeBytes = f.Content, len(f.Content)
		}
		out.Files = append(out.Files, r)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	return out
}

func (rt *Router) handleListAppFiles(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: list app files failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, appFilesResourceFrom(name, svc.Files))
}

// handleSetAppFile upserts one file. A secret file's text goes to the
// secrets store; the app row keeps only a hash, so it is never read back.
func (rt *Router) handleSetAppFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req setAppFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	updater, ok := rt.apps.(appFilesUpdater)
	if !ok {
		writeError(w, http.StatusNotImplemented, "files are not supported by this store")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set app file: load app failed", err, slog.String("name", name))
		return
	}
	clean := path.Clean(req.Path)
	next := store.ServiceFile{Path: clean, Mode: req.Mode, Secret: req.Secret, Hash: store.FileContentHash(name, clean, req.Content)}
	if !req.Secret {
		next.Content = req.Content
	}
	files := append([]store.ServiceFile(nil), svc.Files...)
	replaced := false
	for i, f := range files {
		if f.Path == clean {
			files[i], replaced = next, true
		}
	}
	if !replaced {
		files = append(files, next)
	}
	specFiles := make([]spec.File, len(files))
	for i, f := range files {
		specFiles[i] = spec.File{Path: f.Path, Mode: f.Mode, Content: f.Content}
		if f.Path == clean {
			specFiles[i].Content = req.Content
		}
	}
	if err := spec.ValidateFiles(specFiles, spec.FileLimitsFromEnv()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Secret {
		if rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured")
			return
		}
		if err := rt.secrets.SetValueGuarded(r.Context(), name, store.FileSecretKey(clean), req.Content, true); err != nil {
			rt.internalError(w, "api: set app file: store secret failed", err, slog.String("name", name))
			return
		}
	}
	if err := updater.UpdateServiceFiles(r.Context(), name, files); err != nil {
		rt.internalError(w, "api: set app file failed", err, slog.String("name", name))
		return
	}
	rt.applyFilesIfRequested(r, name)
	writeJSON(w, http.StatusOK, appFilesResourceFrom(name, files))
}

func (rt *Router) handleDeleteAppFile(w http.ResponseWriter, r *http.Request) {
	name, target := r.PathValue("name"), path.Clean(r.URL.Query().Get("path"))
	updater, ok := rt.apps.(appFilesUpdater)
	if !ok {
		writeError(w, http.StatusNotImplemented, "files are not supported by this store")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete app file: load app failed", err, slog.String("name", name))
		return
	}
	kept := make([]store.ServiceFile, 0, len(svc.Files))
	for _, f := range svc.Files {
		if f.Path != target {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(svc.Files) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err := updater.UpdateServiceFiles(r.Context(), name, kept); err != nil {
		rt.internalError(w, "api: delete app file failed", err, slog.String("name", name))
		return
	}
	rt.applyFilesIfRequested(r, name)
	w.WriteHeader(http.StatusNoContent)
}

// applyFilesIfRequested recreates the container when ?apply=restart is set,
// the same restart a pending config change offers; otherwise the change
// shows as pending until the next restart or deploy.
func (rt *Router) applyFilesIfRequested(r *http.Request, name string) {
	if r.URL.Query().Get("apply") != pendingApplyRestart {
		rt.nudgeReconciler()
		return
	}
	if err := rt.apps.RestartService(r.Context(), name); err != nil {
		rt.logger.Warn("api: restart after file change failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}
