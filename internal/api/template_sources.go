package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/catalog"
	"github.com/glincker/levelrail/internal/catalog/sources"
	"github.com/glincker/levelrail/internal/envredact"
	"github.com/glincker/levelrail/internal/gitsshkey"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// TemplateSourceStore keeps private template sources and their validated entries.
type TemplateSourceStore interface {
	CreateTemplateSource(ctx context.Context, s store.TemplateSource) error
	GetTemplateSource(ctx context.Context, name string) (store.TemplateSource, error)
	ListTemplateSources(ctx context.Context) ([]store.TemplateSource, error)
	DeleteTemplateSource(ctx context.Context, name string) error
	ReplaceTemplateSourceEntries(ctx context.Context, name, commit string, entries []store.TemplateSourceEntry) error
	SetTemplateSourceStatus(ctx context.Context, name, status, detail string) error
	ListTemplateSourceEntries(ctx context.Context, name string) ([]store.TemplateSourceEntry, error)
	ApproveTemplateSourceEntry(ctx context.Context, source, id, hash string) error
}

type templateSourceRequest struct {
	Name         string `json:"name"`
	GitURL       string `json:"git_url"`
	Ref          string `json:"ref"`
	DeployKeyApp string `json:"deploy_key_app"`
}

// templateSourceLimits reads the live caps; a missing settings service uses the registry defaults.
func (rt *Router) templateSourceLimits(ctx context.Context) sources.Limits {
	get := func(key string) int {
		if rt.runtimeSettings != nil {
			return rt.runtimeSettings.Int(ctx, key)
		}
		return 0
	}
	return sources.Limits{
		MaxBytes:     int64(get(runtimesettings.KeyTemplatesSourcesMaxMB)) << 20,
		MaxWireBytes: int64(get(runtimesettings.KeyTemplatesSourcesMaxWireMB)) << 20,
		MaxFiles:     get(runtimesettings.KeyTemplatesSourcesMaxFiles),
		MaxDepth:     get(runtimesettings.KeyTemplatesSourcesMaxDepth),
		Timeout:      time.Duration(get(runtimesettings.KeyTemplatesSourcesTimeoutSeconds)) * time.Second,
	}
}

// templateSourcesReady answers 501 without a store and 403 while the setting is off.
func (rt *Router) templateSourcesReady(w http.ResponseWriter, r *http.Request) bool {
	if rt.templateSources == nil {
		writeError(w, http.StatusNotImplemented, "template sources are not configured")
		return false
	}
	if rt.runtimeSettings == nil || !rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyTemplatesSourcesEnabled) {
		writeError(w, http.StatusForbidden, "template sources are off: turn on templates.sources_enabled in settings first")
		return false
	}
	return true
}

// handleListTemplateSources handles GET /api/v1/templates/sources.
func (rt *Router) handleListTemplateSources(w http.ResponseWriter, r *http.Request) {
	if rt.templateSources == nil {
		writeJSON(w, http.StatusOK, []store.TemplateSource{})
		return
	}
	rows, err := rt.templateSources.ListTemplateSources(r.Context())
	if err != nil {
		rt.internalError(w, "api: list template sources failed", err)
		return
	}
	if rows == nil {
		rows = []store.TemplateSource{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// handleCreateTemplateSource handles POST /api/v1/templates/sources. It records the
// source only; the first fetch is an explicit sync.
func (rt *Router) handleCreateTemplateSource(w http.ResponseWriter, r *http.Request) {
	if !rt.templateSourcesReady(w, r) {
		return
	}
	var req templateSourceRequest
	if !decodeReq(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.GitURL = strings.TrimSpace(req.GitURL)
	if !sources.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be lowercase letters, digits and dashes (1 to 40 characters)")
		return
	}
	if !sources.ValidRef(req.Ref) {
		writeError(w, http.StatusBadRequest, "ref must be a plain branch name")
		return
	}
	ssh, err := sources.CheckURL(r.Context(), req.GitURL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if ssh && req.DeployKeyApp == "" {
		writeError(w, http.StatusBadRequest, "an ssh source needs deploy_key_app, the app whose stored deploy key to use")
		return
	}
	src := store.TemplateSource{Name: req.Name, GitURL: req.GitURL, Ref: req.Ref, DeployKeyApp: req.DeployKeyApp}
	if err := rt.templateSources.CreateTemplateSource(r.Context(), src); err != nil {
		if errors.Is(err, store.ErrTemplateSourceExists) {
			writeError(w, http.StatusConflict, "a template source with this name already exists")
			return
		}
		rt.internalError(w, "api: create template source failed", err)
		return
	}
	got, err := rt.templateSources.GetTemplateSource(r.Context(), req.Name)
	if err != nil {
		rt.internalError(w, "api: read template source failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

// handleListTemplateSourceEntries handles GET /api/v1/templates/sources/{name}/templates.
func (rt *Router) handleListTemplateSourceEntries(w http.ResponseWriter, r *http.Request) {
	if rt.templateSources == nil {
		writeJSON(w, http.StatusOK, []store.TemplateSourceEntry{})
		return
	}
	name := r.PathValue("name")
	if _, err := rt.templateSources.GetTemplateSource(r.Context(), name); err != nil {
		rt.templateSourceError(w, err)
		return
	}
	rows, err := rt.templateSources.ListTemplateSourceEntries(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list template source entries failed", err)
		return
	}
	if rows == nil {
		rows = []store.TemplateSourceEntry{}
	}
	writeJSON(w, http.StatusOK, rows)
}

type templateSourceSyncResult struct {
	Source  store.TemplateSource `json:"source"`
	Skipped []string             `json:"skipped"`
}

// handleSyncTemplateSource handles POST /api/v1/templates/sources/{name}/sync: it
// fetches the repo under the live caps and keeps only templates that validate.
func (rt *Router) handleSyncTemplateSource(w http.ResponseWriter, r *http.Request) {
	if !rt.templateSourcesReady(w, r) {
		return
	}
	ctx := r.Context()
	name := r.PathValue("name")
	src, err := rt.templateSources.GetTemplateSource(ctx, name)
	if err != nil {
		rt.templateSourceError(w, err)
		return
	}
	in := sources.Source{Name: src.Name, URL: src.GitURL, Ref: src.Ref}
	if gitsshkey.IsSSHURL(src.GitURL) {
		auth, aerr := gitsshkey.DeployKeyAuth(ctx, rt.gitSourceSecrets, store.GitSourceSecretsKey(src.DeployKeyApp))
		if aerr != nil {
			rt.failTemplateSourceSync(ctx, w, name, aerr)
			return
		}
		in.Auth = auth
	}
	fsys, commit, err := sources.Fetch(ctx, in, rt.templateSourceLimits(ctx), nil)
	if err != nil {
		rt.failTemplateSourceSync(ctx, w, name, err)
		return
	}
	res := sources.Build(name, fsys, sources.IsBuiltin)
	if err := rt.templateSources.ReplaceTemplateSourceEntries(ctx, name, commit, res.Entries); err != nil {
		rt.internalError(w, "api: store template source entries failed", err)
		return
	}
	status, detail := "ok", ""
	if len(res.Skipped) > 0 {
		status, detail = "partial", strings.Join(res.Skipped, "; ")
		if len(detail) > 2000 {
			detail = detail[:2000]
		}
	}
	if err := rt.templateSources.SetTemplateSourceStatus(ctx, name, status, detail); err != nil {
		rt.internalError(w, "api: record template source status failed", err)
		return
	}
	got, err := rt.templateSources.GetTemplateSource(ctx, name)
	if err != nil {
		rt.internalError(w, "api: read template source failed", err)
		return
	}
	if res.Skipped == nil {
		res.Skipped = []string{}
	}
	writeJSON(w, http.StatusOK, templateSourceSyncResult{Source: got, Skipped: res.Skipped})
}

// installSourceTemplate installs a synced source entry (id is source/name) through
// installTemplate. It refuses while sources are off or the entry awaits review.
func (rt *Router) installSourceTemplate(w http.ResponseWriter, r *http.Request, id string) {
	if !rt.templateSourcesReady(w, r) {
		return
	}
	source, _, _ := strings.Cut(id, "/")
	entries, err := rt.templateSources.ListTemplateSourceEntries(r.Context(), source)
	if err != nil {
		rt.internalError(w, "api: install source template: list entries failed", err)
		return
	}
	for _, e := range entries {
		if e.ID != id {
			continue
		}
		if e.Review != store.TemplateReviewApproved {
			writeError(w, http.StatusConflict, "this template changed since it was approved: review the source and approve it before installing")
			return
		}
		m, err := catalog.ManifestFromSource(e.TemplateYAML, e.ComposeYAML)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "this source template no longer passes validation: "+err.Error())
			return
		}
		tpl, err := m.Template()
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		tpl.ID = id
		tpl.Manifest.ID = id
		rt.installTemplate(w, r, tpl)
		return
	}
	writeError(w, http.StatusNotFound, "service template not found")
}

type templateSourceApproveRequest struct {
	ContentHash string `json:"content_hash"`
}

// TemplateSourceEntryDetail is what root reviews before approving: the entry's files with
// secret-looking values masked, and the hash an approval must carry.
type TemplateSourceEntryDetail struct {
	ID          string `json:"id"`
	ContentHash string `json:"content_hash"`
	Review      string `json:"review"`
	Manifest    string `json:"manifest"`
	Compose     string `json:"compose"`
}

var templateSourceSecretLine = regexp.MustCompile(`^(\s*(?:-\s*)?["']?)([A-Za-z_][A-Za-z0-9_]*)(["']?\s*[:=]\s*)(.+)$`)

// maskTemplateSourceText hides inline values of secret-looking keys; ${VAR} references stay.
func maskTemplateSourceText(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		m := templateSourceSecretLine.FindStringSubmatch(l)
		if m == nil || strings.HasPrefix(strings.TrimSpace(m[4]), "$") {
			continue
		}
		if masked, meta := envredact.Value(m[2], strings.Trim(strings.TrimSpace(m[4]), `"'`), nil); meta.ValueRedacted {
			lines[i] = m[1] + m[2] + m[3] + masked
		}
	}
	return strings.Join(lines, "\n")
}

// handleGetTemplateSourceEntry handles GET /api/v1/templates/sources/{name}/templates/{id}.
func (rt *Router) handleGetTemplateSourceEntry(w http.ResponseWriter, r *http.Request) {
	if rt.templateSources == nil {
		writeError(w, http.StatusNotImplemented, "template sources are not configured")
		return
	}
	name := r.PathValue("name")
	id := name + "/" + r.PathValue("id")
	rows, err := rt.templateSources.ListTemplateSourceEntries(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: read template source entry failed", err)
		return
	}
	for _, e := range rows {
		if e.ID == id {
			writeJSON(w, http.StatusOK, TemplateSourceEntryDetail{
				ID: e.ID, ContentHash: e.ContentHash, Review: e.Review,
				Manifest: maskTemplateSourceText(e.TemplateYAML), Compose: maskTemplateSourceText(e.ComposeYAML),
			})
			return
		}
	}
	writeError(w, http.StatusNotFound, "template source entry not found")
}

// handleApproveTemplateSourceEntry handles POST /api/v1/templates/sources/{name}/templates/{id}/approve.
func (rt *Router) handleApproveTemplateSourceEntry(w http.ResponseWriter, r *http.Request) {
	if !rt.templateSourcesReady(w, r) {
		return
	}
	const hashMsg = "content_hash is required: read the entry first and approve the hash you reviewed"
	req, ok := decodeJSON[templateSourceApproveRequest](w, r, 1<<16, decodeMessage(hashMsg))
	if !ok {
		return
	}
	if req.ContentHash == "" {
		writeError(w, http.StatusBadRequest, hashMsg)
		return
	}
	name := r.PathValue("name")
	if err := rt.templateSources.ApproveTemplateSourceEntry(r.Context(), name, name+"/"+r.PathValue("id"), req.ContentHash); err != nil {
		if errors.Is(err, store.ErrTemplateSourceHashMismatch) {
			writeError(w, http.StatusConflict, "the entry changed since you reviewed it: read it again and approve the new hash")
			return
		}
		rt.templateSourceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) failTemplateSourceSync(ctx context.Context, w http.ResponseWriter, name string, err error) {
	msg := err.Error()
	if errors.Is(err, gitsshkey.ErrAuthRequired) {
		msg = "the source needs a stored ssh deploy key on deploy_key_app"
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_ = rt.templateSources.SetTemplateSourceStatus(ctx, name, "error", msg)
	writeError(w, http.StatusBadGateway, "sync failed: "+msg)
}

// handleDeleteTemplateSource handles DELETE /api/v1/templates/sources/{name}.
func (rt *Router) handleDeleteTemplateSource(w http.ResponseWriter, r *http.Request) {
	if rt.templateSources == nil {
		writeError(w, http.StatusNotImplemented, "template sources are not configured")
		return
	}
	if err := rt.templateSources.DeleteTemplateSource(r.Context(), r.PathValue("name")); err != nil {
		rt.templateSourceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) templateSourceError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrTemplateSourceNotFound) {
		writeError(w, http.StatusNotFound, "template source not found")
		return
	}
	rt.internalError(w, "api: template source failed", err)
}
