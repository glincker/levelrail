package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const inlineURLPrefix = "inline://"

// AuditActionInlineSourceDeploy records a deploy from pasted Dockerfile text by hash and size, never the text.
const AuditActionInlineSourceDeploy = "app.deploy.inline_source"

// inlineSourceInput is a pasted Dockerfile plus optional extra build context files.
type inlineSourceInput struct {
	Dockerfile string            `json:"dockerfile"`
	Files      []inlineFileInput `json:"files,omitempty"`
}

type inlineFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// InlineSourceStore keeps inline sources by content hash so queued and retried builds rebuild the same text.
type InlineSourceStore interface {
	SaveInlineSource(ctx context.Context, src store.InlineSource) (string, error)
	GetInlineSource(ctx context.Context, serviceName, hash string) (*store.InlineSource, error)
	PruneInlineSources(ctx context.Context, keep int) (int64, error)
}

type inlineMeta struct {
	hash  string
	size  int64
	files int
}

// inlineSources reuses the git source store, which is the same SQLite handle in every real router.
func (rt *Router) inlineSources() InlineSourceStore {
	st, _ := rt.gitSources.(InlineSourceStore)
	return st
}

// prepareInlineBuild stores req.InlineSource and points req at it, dropping
// the text from req so a queued build body never carries it.
func (rt *Router) prepareInlineBuild(r *http.Request, name string, req *triggerBuildRequest, buildType string) (inlineMeta, int, string) {
	in := req.InlineSource
	switch {
	case buildType != spec.BuildDockerfile:
		return inlineMeta{}, http.StatusBadRequest, fmt.Sprintf("inline_source needs build.type %q, not %q", spec.BuildDockerfile, buildType)
	case req.RepoURL != "" || req.FromCache || req.UploadHash != "":
		return inlineMeta{}, http.StatusBadRequest, "inline_source cannot be combined with repo_url, upload_hash or from_cache"
	case req.Build.BaseDirectory != "" || (req.Build.Path != "" && req.Build.Path != build.InlineDockerfileName):
		return inlineMeta{}, http.StatusBadRequest, "build.path and build.base_directory are not meaningful with inline_source"
	case strings.TrimSpace(in.Dockerfile) == "":
		return inlineMeta{}, http.StatusBadRequest, "inline_source.dockerfile is required"
	}
	st := rt.inlineSources()
	if st == nil {
		return inlineMeta{}, http.StatusNotImplemented, "inline sources are not configured on this control plane"
	}
	files := make([]store.InlineFile, 0, len(in.Files))
	for _, f := range in.Files {
		files = append(files, store.InlineFile{Path: f.Path, Content: f.Content})
	}
	maxFiles := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyInlineSourceMaxFiles)
	if len(files) > maxFiles {
		return inlineMeta{}, http.StatusRequestEntityTooLarge, fmt.Sprintf("inline_source has %d files, the limit is %d (%s)", len(files), maxFiles, runtimesettings.KeyInlineSourceMaxFiles)
	}
	size := store.InlineSourceSize(in.Dockerfile, files)
	if maxBytes := int64(rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyInlineSourceMaxBytes)); size > maxBytes {
		return inlineMeta{}, http.StatusRequestEntityTooLarge, fmt.Sprintf("inline_source is %d bytes, the limit is %d (%s)", size, maxBytes, runtimesettings.KeyInlineSourceMaxBytes)
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	if err := build.ValidateInlineFileSet(paths); err != nil {
		return inlineMeta{}, http.StatusBadRequest, "inline_source.files: " + err.Error()
	}
	hash, err := st.SaveInlineSource(r.Context(), store.InlineSource{ServiceName: name, Dockerfile: in.Dockerfile, Files: files})
	if err != nil {
		rt.logger.Error("api: save inline source failed", slog.String("error", err.Error()), slog.String("name", name))
		return inlineMeta{}, http.StatusInternalServerError, "could not store the inline source"
	}
	meta := inlineMeta{hash: hash, size: size, files: len(files)}
	req.InlineSource = nil
	req.RepoURL = inlineURLPrefix + hash
	req.Ref = hash[:40]
	req.Build.Path = build.InlineDockerfileName
	return meta, 0, ""
}

// CleanupInlineSources drops each app's older inline sources beyond the live keep setting.
func (rt *Router) CleanupInlineSources(ctx context.Context) {
	st := rt.inlineSources()
	if st == nil {
		return
	}
	n, err := st.PruneInlineSources(ctx, rt.runtimeSettings.Limit(ctx, runtimesettings.KeyInlineSourceKeep))
	if err != nil {
		rt.logger.Warn("api: inline source cleanup failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		rt.logger.InfoContext(ctx, "api: old inline sources removed", slog.Int64("removed", n))
	}
}

// inlineMetaFor rebuilds the commit metadata of a queued inline build from the stored source.
func (rt *Router) inlineMetaFor(ctx context.Context, app, repoURL string) (inlineMeta, bool) {
	st := rt.inlineSources()
	if st == nil {
		return inlineMeta{}, false
	}
	src, err := st.GetInlineSource(ctx, app, strings.TrimPrefix(repoURL, inlineURLPrefix))
	if err != nil {
		return inlineMeta{}, false
	}
	return inlineMeta{hash: src.Hash, size: src.SizeBytes, files: len(src.Files)}, true
}

// fetchInline writes a stored inline source into a private temp build context.
func (rt *Router) fetchInline(ctx context.Context, app, repoURL string) (dir, commit string, cleanup func(), err error) {
	st := rt.inlineSources()
	if st == nil {
		return "", "", nil, errors.New("inline sources are not configured")
	}
	hash := strings.TrimPrefix(repoURL, inlineURLPrefix)
	src, err := st.GetInlineSource(ctx, app, hash)
	if err != nil {
		return "", "", nil, fmt.Errorf("load inline source %s: %w", hash, err)
	}
	files := make([]build.InlineFile, 0, len(src.Files))
	for _, f := range src.Files {
		files = append(files, build.InlineFile{Path: f.Path, Content: f.Content})
	}
	dir, cleanup, err = build.MaterializeInlineContext(src.Dockerfile, files)
	if err != nil {
		return "", "", nil, fmt.Errorf("materialize inline source %s: %w", hash, err)
	}
	return dir, hash[:40], cleanup, nil
}

func withInlineCommitMeta(ctx context.Context, m inlineMeta) context.Context {
	return context.WithValue(ctx, commitMetaKey{}, commitMeta{Message: fmt.Sprintf("Inline Dockerfile, %d extra files, %s", m.files, m.hash[:12])})
}

// auditInlineSource runs only once the deploy is accepted, so a refused request leaves no success entry.
func (rt *Router) auditInlineSource(r *http.Request, app string, m inlineMeta) {
	if rt.auditLog == nil || m.hash == "" {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: inline source audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: http.MethodPost, Path: "/api/v1/apps/" + app + "/builds", StatusCode: http.StatusAccepted,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionInlineSourceDeploy, Detail: fmt.Sprintf(`{"sha256":%q,"bytes":%d,"files":%d}`, m.hash, m.size, m.files),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: inline source audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
