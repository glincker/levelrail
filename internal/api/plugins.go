package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/store"
)

const pluginBodyLimit = 128 << 10

// SetPluginManager enables the plugin routes and plugin channel kinds.
func (rt *Router) SetPluginManager(m *plugin.Manager) { rt.plugins = m }

type pluginResource struct {
	plugin.Record
	Running      bool           `json:"running"`
	Health       string         `json:"health"`
	Verification string         `json:"verification"`
	Fix          string         `json:"fix"`
	Panels       []plugin.Panel `json:"panels"`
}

type installPluginRequest struct {
	Manifest         string   `json:"manifest"`
	ManifestURL      string   `json:"manifest_url,omitempty"`
	Signature        string   `json:"signature"`
	SignatureURL     string   `json:"signature_url,omitempty"`
	KeyID            string   `json:"key_id"`
	ExpectedDigest   string   `json:"expected_digest,omitempty"`
	ConfigJSON       string   `json:"config_json,omitempty"`
	ApproveAbilities []string `json:"approve_abilities,omitempty"`
}

// installRequest resolves a manifest URL, if given, into manifest and signature text.
func (rt *Router) installRequest(ctx context.Context, req installPluginRequest) (plugin.InstallRequest, error) {
	manifest, signature := req.Manifest, strings.TrimSpace(req.Signature)
	if strings.TrimSpace(req.ManifestURL) != "" {
		if manifest != "" {
			return plugin.InstallRequest{}, &plugin.InvalidError{Msg: "give either manifest or manifest_url, not both"}
		}
		fetch := rt.pluginFetch
		if fetch == nil {
			fetch = plugin.Fetch
		}
		var err error
		if manifest, signature, err = plugin.LoadSource(ctx, fetch, req.ManifestURL, req.SignatureURL); err != nil {
			return plugin.InstallRequest{}, err
		}
		if s := strings.TrimSpace(req.Signature); s != "" {
			signature = s
		}
	}
	return plugin.InstallRequest{
		Manifest: []byte(manifest), Signature: signature, KeyID: strings.TrimSpace(req.KeyID),
		ExpectedDigest: strings.TrimSpace(req.ExpectedDigest), ConfigJSON: req.ConfigJSON, Approve: req.ApproveAbilities,
	}, nil
}

type approvePluginRequest struct {
	Abilities []string `json:"abilities"`
}

type addPluginKeyRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

func (rt *Router) pluginManager(w http.ResponseWriter) (*plugin.Manager, bool) {
	if rt.plugins == nil {
		writeError(w, http.StatusServiceUnavailable, "the plugin runtime is not available on this control plane (it needs a reachable Docker daemon)")
		return nil, false
	}
	return rt.plugins, true
}

func (rt *Router) pluginActor(r *http.Request) string {
	kind, id, _, ok := rt.currentActor(r)
	if !ok {
		return "unknown"
	}
	return kind + ":" + id
}

func (rt *Router) toPluginResource(ctx context.Context, m *plugin.Manager, rec plugin.Record) pluginResource {
	st := m.StatusOf(ctx, rec)
	panels := []plugin.Panel{}
	if man, err := plugin.ParseManifest([]byte(rec.Manifest)); err == nil {
		panels = man.Contributes.Panels
	}
	return pluginResource{Record: rec, Running: m.Running(rec.Name), Health: st.Health, Verification: st.Verification, Fix: st.Fix, Panels: panels}
}

func (rt *Router) writePluginError(w http.ResponseWriter, what string, err error) {
	var inv *plugin.InvalidError
	switch {
	case errors.As(err, &inv):
		writeError(w, http.StatusBadRequest, inv.Error())
	case errors.Is(err, plugin.ErrNotFound):
		writeError(w, http.StatusNotFound, "plugin not found")
	case errors.Is(err, plugin.ErrExists):
		writeError(w, http.StatusConflict, "a plugin with that name is already installed, remove it first")
	case errors.Is(err, plugin.ErrNotApproved):
		writeError(w, http.StatusConflict, "approve the plugin's abilities before enabling it")
	case errors.Is(err, plugin.ErrState):
		writeError(w, http.StatusConflict, "the plugin is being removed")
	case errors.Is(err, plugin.ErrUntrusted):
		writeError(w, http.StatusUnprocessableEntity, "the signing key is not in the trust list, add the publisher's key first")
	case errors.Is(err, plugin.ErrBadSignature):
		writeError(w, http.StatusUnprocessableEntity, "the signature does not verify for this manifest and image digest")
	case errors.Is(err, plugin.ErrUnsigned):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, plugin.ErrSourceNotFound):
		writeError(w, http.StatusUnprocessableEntity, "nothing was found at that URL")
	case errors.Is(err, plugin.ErrDigestMismatch):
		writeError(w, http.StatusUnprocessableEntity, "the image digest does not match the digest you pinned")
	default:
		rt.internalError(w, "api: plugin "+what+" failed", err)
	}
}

// handleListPlugins serves GET /api/v1/plugins.
func (rt *Router) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	recs, err := m.List(r.Context())
	if err != nil {
		rt.writePluginError(w, "list", err)
		return
	}
	out := make([]pluginResource, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rt.toPluginResource(r.Context(), m, rec))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPlugin serves GET /api/v1/plugins/{name}.
func (rt *Router) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	rec, err := m.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writePluginError(w, "get", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleInstallPlugin serves POST /api/v1/plugins. It verifies and records the package; nothing is approved or started.
func (rt *Router) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req installPluginRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	ir, err := rt.installRequest(r.Context(), req)
	if err != nil {
		rt.writePluginError(w, "install", err)
		return
	}
	rec, err := m.Install(r.Context(), ir, rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "install", err)
		return
	}
	writeJSON(w, http.StatusCreated, rt.toPluginResource(r.Context(), m, rec))
}

// handleInspectPlugin serves POST /api/v1/plugins/inspect. It verifies a package and shows the ability diff, writing nothing.
func (rt *Router) handleInspectPlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req installPluginRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	ir, err := rt.installRequest(r.Context(), req)
	if err != nil {
		rt.writePluginError(w, "inspect", err)
		return
	}
	p, err := m.Inspect(r.Context(), ir)
	if err != nil {
		rt.writePluginError(w, "inspect", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleUpgradePlugin serves POST /api/v1/plugins/{name}/upgrade.
func (rt *Router) handleUpgradePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req installPluginRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	ir, err := rt.installRequest(r.Context(), req)
	if err != nil {
		rt.writePluginError(w, "upgrade", err)
		return
	}
	if man, perr := plugin.ParseManifest(ir.Manifest); perr == nil && man.Name != r.PathValue("name") {
		writeError(w, http.StatusBadRequest, "the manifest is for a different plugin than the one in the URL")
		return
	}
	rec, err := m.Upgrade(r.Context(), ir, rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "upgrade", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleRestartPlugin serves POST /api/v1/plugins/{name}/restart, the fix for an unhealthy plugin.
func (rt *Router) handleRestartPlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	rec, err := m.Restart(r.Context(), r.PathValue("name"), rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "restart", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleApprovePlugin serves POST /api/v1/plugins/{name}/approve.
func (rt *Router) handleApprovePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req approvePluginRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	rec, err := m.Approve(r.Context(), r.PathValue("name"), req.Abilities, rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "approve", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleEnablePlugin serves POST /api/v1/plugins/{name}/enable.
func (rt *Router) handleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	rec, err := m.Enable(r.Context(), r.PathValue("name"), rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "enable", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleDisablePlugin serves POST /api/v1/plugins/{name}/disable.
func (rt *Router) handleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	rec, err := m.Disable(r.Context(), r.PathValue("name"), rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "disable", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toPluginResource(r.Context(), m, rec))
}

// handleRemovePlugin serves DELETE /api/v1/plugins/{name}.
func (rt *Router) handleRemovePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	if err := m.Remove(r.Context(), r.PathValue("name"), rt.pluginActor(r)); err != nil {
		if errors.Is(err, plugin.ErrNotFound) {
			rt.writePluginError(w, "remove", err)
			return
		}
		rt.logger.Error("api: plugin removal needs a retry", "error", err.Error())
		writeError(w, http.StatusAccepted, "the plugin is marked for removal, container cleanup will be retried")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListPluginKeys serves GET /api/v1/plugin-keys.
func (rt *Router) handleListPluginKeys(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	keys, err := m.TrustKeys(r.Context())
	if err != nil {
		rt.writePluginError(w, "list keys", err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// handleAddPluginKey serves POST /api/v1/plugin-keys.
func (rt *Router) handleAddPluginKey(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	var req addPluginKeyRequest
	if err := decodeJSONBodyLimit(r, &req, pluginBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	k, err := m.TrustAdd(r.Context(), req.Name, req.PublicKey, rt.pluginActor(r))
	if err != nil {
		rt.writePluginError(w, "add key", err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// handleRemovePluginKey serves DELETE /api/v1/plugin-keys/{id}.
func (rt *Router) handleRemovePluginKey(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	if err := m.TrustRemove(r.Context(), r.PathValue("id"), rt.pluginActor(r)); err != nil {
		rt.writePluginError(w, "remove key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pluginKindError reports why a plugin:<name> channel kind cannot be used, or nil.
func (rt *Router) pluginKindError(ctx context.Context, kind alerting.NotifyKind) error {
	if rt.plugins == nil {
		return errors.New("plugin channels need the plugins feature and a running plugin runtime")
	}
	rec, err := rt.plugins.Get(ctx, alerting.PluginName(kind))
	if err != nil || rec.Kind != plugin.KindNotificationChannel {
		return errors.New("kind must name an installed notification_channel plugin as plugin:<name>")
	}
	return nil
}

// PluginAuditor writes plugin audit events into the audit log.
type PluginAuditor struct{ DB *store.DB }

// Record implements plugin.Auditor.
func (a PluginAuditor) Record(ctx context.Context, e plugin.AuditEvent) error {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return err
	}
	actorType, actorID, _ := strings.Cut(e.Actor, ":")
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return err
	}
	path := "plugin:" + e.Plugin
	if e.Plugin == "" {
		path = "plugin"
	}
	return a.DB.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorID, Ability: auditAbility(e), Method: "PLUGIN", Path: path,
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", Action: e.Action, Detail: string(detail),
	})
}

func auditAbility(e plugin.AuditEvent) string {
	if a := e.Detail["abilities"]; a != "" {
		return a
	}
	return "root"
}
