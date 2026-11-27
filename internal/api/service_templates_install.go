package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/hostswap"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// installAppName bounds an installed stack's name: it prefixes every
// container, volume and network name the stack owns.
var installAppName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

type templateInstallRequest struct {
	Name    string            `json:"name,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Domains map[string]string `json:"domains,omitempty"`
	// ProjectID groups the installed services under a project; empty uses the switcher's project.
	ProjectID string `json:"project_id,omitempty"`
	// AllowPlatformMismatch installs despite an architecture the template did not verify.
	AllowPlatformMismatch bool `json:"allow_platform_mismatch,omitempty"`
}

func manifestIcon(m *catalog.Manifest) string {
	if m == nil {
		return ""
	}
	return m.Icon
}

// handleInstallServiceTemplate handles POST /api/v1/service-templates/{id}/install:
// renders a manifest template with the operator's prompt values and
// domains, then deploys it as a compose app through deployComposeBody.
// Unlike .../deploy it is not a trial and it accepts configuration.
func (rt *Router) handleInstallServiceTemplate(w http.ResponseWriter, r *http.Request) {
	rec := &compatRecorder{ResponseWriter: w}
	rt.installServiceTemplate(rec, r)
	rt.recordTemplateCompat(r, r.PathValue("id"), rec)
}

func (rt *Router) installServiceTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tpl, ok := catalog.TemplateByID(id)
	if !ok || !templateAvailable(id) {
		writeError(w, http.StatusNotFound, "service template not found")
		return
	}
	if tpl.Manifest == nil {
		writeError(w, http.StatusConflict, "this template has no install manifest, so it cannot be configured at install time; use 'templates deploy "+id+"' (POST /api/v1/service-templates/"+id+"/deploy) instead")
		return
	}
	var req templateInstallRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if rec, ok := w.(*compatRecorder); ok {
		rec.callerName = req.Name
	}
	name := req.Name
	if name == "" {
		generated, err := rt.generateOneClickAppName(r.Context(), tpl.ID)
		if err != nil {
			rt.internalError(w, "api: install template: generate app name failed", err, slog.String("template", tpl.ID))
			return
		}
		name = generated
	}
	if !installAppName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "name must start with a letter and use only lowercase letters, digits and dashes (40 characters at most)")
		return
	}
	if reason, overridable := rt.platformPreflight(r.Context(), tpl); reason != "" && (!overridable || !req.AllowPlatformMismatch) {
		writeError(w, http.StatusUnprocessableEntity, reason)
		return
	}
	if rt.refuseIfTeardownPending(w, r, name) {
		return
	}
	req.ProjectID = defaultProjectFor(r, req.ProjectID)
	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.internalError(w, "api: install template: check project failed", err, slog.String("project_id", req.ProjectID))
		return
	}
	if _, err := rt.appCompose.GetAppByName(r.Context(), name); err == nil {
		writeError(w, http.StatusConflict, "an app named "+name+" already exists")
		return
	} else if !errors.Is(err, store.ErrAppNotFound) {
		rt.internalError(w, "api: install template: check name failed", err, slog.String("name", name))
		return
	}
	if err := tpl.Manifest.CheckFileInputs(req.Env, rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyTemplatesFileInputMaxBytes)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body, err := tpl.Manifest.Render(catalog.InstallInput{Env: req.Env, Domains: req.Domains}, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	grants, ok := rt.resolveTemplateCapabilities(w, r, tpl.Manifest)
	if !ok {
		return
	}
	resp, ok := rt.lockedComposeDeploy(w, r, name, []byte(body), composeDeployOpts{source: store.ComposeRevisionSourceTemplate, templateID: tpl.ID, interpEnv: interpEnvWithoutFiles(tpl.Manifest, req.Env), capGrants: catalog.CapGrantMap(grants)})
	if !ok {
		return
	}
	if n, ok := rt.lowMemoryNotice(tpl.RecommendedMemoryBytes); ok {
		resp.Notices = append(resp.Notices, n)
	}
	if req.ProjectID != "" {
		for _, svc := range resp.Services {
			if err := rt.apps.UpdateServiceProject(r.Context(), svc.Name, req.ProjectID); err != nil {
				rt.logger.Error("api: install template: assign project failed", slog.String("error", err.Error()), slog.String("name", svc.Name), slog.String("project_id", req.ProjectID))
				w.Header().Set("X-Project-Incomplete", "true")
			}
		}
	}
	if !rt.auditCapabilityGrants(r, name, tpl.Manifest, grants) {
		w.Header().Set("X-Audit-Incomplete", "true")
	}
	writeJSON(w, http.StatusOK, resp)
}

// interpEnvWithoutFiles drops file bodies from the install env so a config file
// is never kept as an app env var.
func interpEnvWithoutFiles(m *catalog.Manifest, env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	for _, k := range m.FileInputKeys() {
		delete(out, k)
	}
	return out
}

// lowMemoryNotice is a plain warning, never a block, when a template wants more
// RAM than the host has free.
func (rt *Router) lowMemoryNotice(recommended int64) (composeNoticeResult, bool) {
	if recommended <= 0 {
		return composeNoticeResult{}, false
	}
	read := rt.doctorHostMemory
	if read == nil {
		read = func() (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	_, avail, err := read()
	if err != nil || avail <= 0 || avail >= recommended {
		return composeNoticeResult{}, false
	}
	msg := fmt.Sprintf("This template recommends %s of memory and this host has %s free.", humanBytes(recommended), humanBytes(avail))
	if hostswap.Read("").SwapBytes == 0 {
		msg += " No swap is configured: run the CLI command 'swap' on the control plane host to see how to add a swapfile (nothing changes until you apply it)."
	}
	return composeNoticeResult{Level: string(compose.NoticeLevelWarning), Message: msg}, true
}
