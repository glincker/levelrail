package api

import (
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/repolayout"
)

const maxAffectedFiles = 5000

type affectedRequest struct {
	Branch string   `json:"branch,omitempty"`
	Files  []string `json:"files"`
}

type affectedPackage struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Affected bool   `json:"affected"`
}

type affectedResponse struct {
	// AppDeploys is whether a push touching Files redeploys this app under
	// its configured deploy paths.
	AppDeploys bool              `json:"app_deploys"`
	Reason     string            `json:"reason,omitempty"`
	Packages   []affectedPackage `json:"packages"`
}

// handleGitSourceAffected handles POST /api/v1/apps/{name}/git-source/affected:
// a dry run of which workspace packages, and this app, a changed file list
// would redeploy.
func (rt *Router) handleGitSourceAffected(w http.ResponseWriter, r *http.Request) {
	var req affectedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Files) == 0 || len(req.Files) > maxAffectedFiles {
		writeError(w, http.StatusBadRequest, "files must list between 1 and 5000 changed paths")
		return
	}
	gs, _, res, ok := rt.detectRepoLayout(w, r, r.PathValue("name"), req.Branch)
	if !ok {
		return
	}
	hit := map[string]bool{}
	for _, d := range repolayout.Affected(req.Files, res.Packages) {
		hit[d] = true
	}
	out := affectedResponse{Packages: make([]affectedPackage, 0, len(res.Packages))}
	for _, p := range res.Packages {
		out.Packages = append(out.Packages, affectedPackage{Name: p.Name, Dir: p.Dir, Affected: hit[p.Dir]})
	}
	verdict := deployPathFilter(*gs).Apply(req.Files)
	out.AppDeploys, out.Reason = verdict.Run, verdict.Reason
	writeJSON(w, http.StatusOK, out)
}
