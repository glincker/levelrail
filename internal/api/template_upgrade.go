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

// templateUpgradeState is a freshly computed plan plus what applying it needs.
type templateUpgradeState struct {
	plan   templateupgrade.Plan
	latest store.ComposeRevision
	tpl    string
}

// templateUpgradeErr carries the HTTP status for a state that cannot be built.
type templateUpgradeErr struct {
	status int
	msg    string
	cause  error
}

func (e *templateUpgradeErr) Error() string { return e.msg }
func (e *templateUpgradeErr) Unwrap() error { return e.cause }

func (rt *Router) loadTemplateUpgrade(r *http.Request, appID string) (templateUpgradeState, *templateUpgradeErr) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusNotImplemented, msg: "compose history is not available on this store"}
	}
	latest, err := revs.GetComposeRevision(r.Context(), appID, 0)
	if errors.Is(err, store.ErrComposeRevisionNotFound) {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusNotFound, msg: "this app has no compose history"}
	}
	if err != nil {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusInternalServerError, msg: "api: template upgrade: load latest failed", cause: err}
	}
	tplID := latest.TemplateID
	if tplID == "" {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusNotFound, msg: "this app was not deployed from a catalog template"}
	}
	tpl, ok := catalog.TemplateByID(tplID)
	if !ok {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusNotFound, msg: "the catalog no longer has this template"}
	}
	installed, err := compose.Parse([]byte(latest.Compose))
	if err != nil {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusInternalServerError, msg: "api: template upgrade: parse installed compose failed", cause: err}
	}
	target, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		return templateUpgradeState{}, &templateUpgradeErr{status: http.StatusInternalServerError, msg: "api: template upgrade: parse catalog compose failed", cause: err}
	}
	plan := templateupgrade.Build(templateupgrade.Input{
		AppID: appID, TemplateID: tplID, Installed: installed, Baseline: rt.templateBaseline(r, revs, appID, tplID), Template: target,
	})
	return templateUpgradeState{plan: plan, latest: latest, tpl: tpl.Compose}, nil
}

func (rt *Router) writeTemplateUpgradeErr(w http.ResponseWriter, appID string, e *templateUpgradeErr) {
	if e.status == http.StatusInternalServerError {
		rt.internalError(w, e.msg, e.cause, slog.String("app", appID))
		return
	}
	writeError(w, e.status, e.msg)
}

// templateUpgradePlanResponse adds the dashboard's recheck cadence to the plan.
type templateUpgradePlanResponse struct {
	templateupgrade.Plan
	RecheckSeconds int `json:"recheck_seconds"`
}

// handleGetTemplateUpgrade handles GET /api/v1/apps/{name}/template-upgrade:
// a read-only plan comparing the deployed compose with the current catalog
// template. It carries names and tags only, never env values.
func (rt *Router) handleGetTemplateUpgrade(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("name")
	st, e := rt.loadTemplateUpgrade(r, appID)
	if e != nil {
		rt.writeTemplateUpgradeErr(w, appID, e)
		return
	}
	writeJSON(w, http.StatusOK, templateUpgradePlanResponse{Plan: st.plan, RecheckSeconds: int(templateUpgradeSettingsFromEnv().recheck.Seconds())})
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
