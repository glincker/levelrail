package api

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// installAppName bounds an installed stack's name: it prefixes every
// container, volume and network name the stack owns.
var installAppName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

type templateInstallRequest struct {
	Name    string            `json:"name,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Domains map[string]string `json:"domains,omitempty"`
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
	id := r.PathValue("id")
	tpl, ok := catalog.TemplateByID(id)
	if !ok || !templateAvailable(id) {
		writeError(w, http.StatusNotFound, "service template not found")
		return
	}
	if tpl.Manifest == nil {
		writeError(w, http.StatusConflict, "this template has no install manifest; deploy it through the compose flow instead")
		return
	}
	var req templateInstallRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
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
	if _, err := rt.appCompose.GetAppByName(r.Context(), name); err == nil {
		writeError(w, http.StatusConflict, "an app named "+name+" already exists")
		return
	} else if !errors.Is(err, store.ErrAppNotFound) {
		rt.internalError(w, "api: install template: check name failed", err, slog.String("name", name))
		return
	}
	body, err := tpl.Manifest.Render(catalog.InstallInput{Env: req.Env, Domains: req.Domains}, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, ok := rt.deployComposeBody(w, r, name, []byte(body), composeDeployOpts{source: store.ComposeRevisionSourceTemplate, templateID: tpl.ID})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
