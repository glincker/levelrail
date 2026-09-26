package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/importplan"
)

// importPlanBodyLimit bounds the pasted input, compose files included.
const importPlanBodyLimit = 2 * importplan.MaxInputBytes

// publicRailpackHosts are the only hosts the Railpack detection fallback
// clones from, since build.Detect does not use the SSRF-guarded client.
var publicRailpackHosts = map[string]bool{"github.com": true, "gitlab.com": true, "bitbucket.org": true, "codeberg.org": true}

// handleImportPlan handles POST /api/v1/imports/plan: it classifies the
// input and returns a deployment plan preview. Nothing is created; a deploy
// uses the existing create, build and compose endpoints from the plan.
func (rt *Router) handleImportPlan(w http.ResponseWriter, r *http.Request) {
	var in importplan.Input
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, importPlanBodyLimit)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	deps := importplan.Deps{Detect: rt.importDetect}
	if rt.importFiles != nil {
		deps.Files = rt.importFiles()
	}
	plan, err := importplan.Plan(r.Context(), in, deps)
	if err != nil {
		if errors.Is(err, importplan.ErrEmptyInput) || errors.Is(err, importplan.ErrUnclassified) || strings.HasPrefix(err.Error(), "importplan:") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.logger.Error("api: import plan failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (rt *Router) importDetect(ctx context.Context, repoURL, ref string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil || !publicRailpackHosts[strings.ToLower(u.Hostname())] || rt.detect == nil {
		return "", nil
	}
	res, err := rt.detect(ctx, build.DetectRequest{RepoURL: repoURL, Ref: ref})
	if err != nil || res == nil {
		return "", nil
	}
	return res.Provider, nil
}
