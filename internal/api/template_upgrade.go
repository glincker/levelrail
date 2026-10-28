package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/templateupgrade"
)

// templateBaselineScan bounds how far back we look for the first template revision.
const templateBaselineScan = 200

// handleGetTemplateUpgrade handles GET /api/v1/apps/{name}/template-upgrade:
// a read-only plan comparing the deployed compose with the current catalog
// template. It carries names and tags only, never env values.
func (rt *Router) handleGetTemplateUpgrade(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	appID := r.PathValue("name")
	latest, err := revs.GetComposeRevision(r.Context(), appID, 0)
	if errors.Is(err, store.ErrComposeRevisionNotFound) {
		writeError(w, http.StatusNotFound, "this app has no compose history")
		return
	}
	if err != nil {
		rt.internalError(w, "api: template upgrade: load latest failed", err, slog.String("app", appID))
		return
	}
	tplID := latest.TemplateID
	if tplID == "" {
		writeError(w, http.StatusNotFound, "this app was not deployed from a catalog template")
		return
	}
	tpl, ok := catalog.TemplateByID(tplID)
	if !ok {
		writeError(w, http.StatusNotFound, "the catalog no longer has this template")
		return
	}
	installed, err := compose.Parse([]byte(latest.Compose))
	if err != nil {
		rt.internalError(w, "api: template upgrade: parse installed compose failed", err, slog.String("app", appID))
		return
	}
	target, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		rt.internalError(w, "api: template upgrade: parse catalog compose failed", err, slog.String("template", tplID))
		return
	}
	writeJSON(w, http.StatusOK, templateupgrade.Build(templateupgrade.Input{
		AppID: appID, TemplateID: tplID, Installed: installed, Baseline: rt.templateBaseline(r, revs, appID, tplID), Template: target,
	}))
}

// templateBaseline is the app's first template revision, the closest thing
// to "what the template produced"; nil when history is missing or unparsable.
func (rt *Router) templateBaseline(r *http.Request, revs ComposeRevisionStore, appID, tplID string) *compose.File {
	list, err := revs.ListComposeRevisions(r.Context(), appID, templateBaselineScan)
	if err != nil {
		return nil
	}
	first := 0
	for _, rev := range list {
		if rev.Source == store.ComposeRevisionSourceTemplate && rev.TemplateID == tplID && (first == 0 || rev.Revision < first) {
			first = rev.Revision
		}
	}
	if first == 0 {
		return nil
	}
	rev, err := revs.GetComposeRevision(r.Context(), appID, first)
	if err != nil {
		return nil
	}
	f, err := compose.Parse([]byte(rev.Compose))
	if err != nil {
		return nil
	}
	return f
}
