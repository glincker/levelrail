package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/docs/ci-examples"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ciDeployerAbilities is the least privilege a stored CI token needs: read the app and deploy it.
var ciDeployerAbilities = []string{AbilityRead, AbilityDeploy}

type ciTemplateResource struct {
	ID string `json:"id"`
	// Target is the repository path the file belongs at.
	Target string `json:"target"`
	// Auth is "oidc" (no stored secret, needs a trust) or "token" (needs a stored deployer token).
	Auth    string `json:"auth"`
	Content string `json:"content"`
}

type ciDeployerScope struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
	TeamRole  string `json:"team_role"`
}

type ciTemplatesResource struct {
	Templates []ciTemplateResource `json:"templates"`
	APIURL    string               `json:"api_url"`
	// NeedsDomain is true when no dashboard URL is set, so APIURL is empty and CI cannot reach the control plane yet.
	NeedsDomain bool `json:"needs_domain"`
	// DeployerScope, when set, limits the deployer token to this app's project. Absent means the token is install-wide.
	DeployerScope *ciDeployerScope `json:"deployer_scope,omitempty"`
	// DeployerAbilities are the abilities to grant the stored deployer token for the "token" templates.
	DeployerAbilities []string `json:"deployer_abilities"`
}

// handleGetCITemplates handles GET /api/v1/apps/{name}/ci/templates: the example pipeline for every supported CI,
// with this app's name filled in. The files are the ones docs/ci-examples publishes.
func (rt *Router) handleGetCITemplates(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireAppForCI(w, r, name) {
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "ci templates: load app", err)
		return
	}
	apiURL, err := rt.ingressSettings.GetDashboardURL(r.Context())
	if err != nil {
		rt.internalError(w, "ci templates: dashboard url", err)
		return
	}
	list, err := ciexamples.Templates()
	if err != nil {
		rt.internalError(w, "ci templates: manifest", err)
		return
	}
	scope := rt.ciDeployerScopeFor(r.Context(), svc.ProjectID)
	out := ciTemplatesResource{DeployerScope: scope, Templates: make([]ciTemplateResource, 0, len(list)), APIURL: apiURL, NeedsDomain: apiURL == "", DeployerAbilities: ciDeployerAbilities}
	for _, t := range list {
		content, err := ciexamples.Content(t, name)
		if err != nil {
			rt.internalError(w, "ci templates: read", err)
			return
		}
		out.Templates = append(out.Templates, ciTemplateResource{ID: t.ID, Target: t.Target, Auth: t.Auth, Content: content})
	}
	writeJSON(w, http.StatusOK, out)
}

// ciDeployerScopeFor limits a deployer token to the app's project when it has one and teams are available.
func (rt *Router) ciDeployerScopeFor(ctx context.Context, projectID string) *ciDeployerScope {
	db := rt.teamDB()
	if projectID == "" || db == nil {
		return nil
	}
	team, err := db.TeamOfProject(ctx, projectID)
	if err != nil || team == "" {
		return nil
	}
	return &ciDeployerScope{TeamID: team, ProjectID: projectID, TeamRole: store.TeamRoleDeveloper}
}
