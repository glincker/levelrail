package api

import "net/http"

const (
	riskRead      = "read"
	riskWrite     = "write"
	riskSensitive = "sensitive"
	riskRoot      = "root"
)

type abilityInfo struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Risk        string `json:"risk"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var abilityCatalog = []abilityInfo{
	{AbilityRead, "view", riskRead, "Read", "See apps, databases, deploys, logs and settings. Secret values stay hidden."},
	{AbilityReadSensitive, "view", riskSensitive, "Read sensitive", "Reveal secret values such as env vars, connection strings and backups."},
	{AbilityWrite, "change", riskWrite, "Write", "Create and change apps and databases, restart, scale and edit settings."},
	{AbilityWriteSensitive, "change", riskSensitive, "Write sensitive", "Change secrets, domains, access rules and other security relevant settings."},
	{AbilityDeploy, "deploy", riskWrite, "Deploy", "Ship a new version, roll back and approve deploys."},
	{AbilityRoot, "admin", riskRoot, "Root", "Everything, including users, tokens, policies and nodes."},
}

func abilityRisk(id string) string {
	for _, a := range abilityCatalog {
		if a.ID == id {
			return a.Risk
		}
	}
	return riskRead
}

type resourceKindInfo struct {
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
}

// conditionSupport states plainly that the evaluator has no condition engine:
// the only supported narrowing is an environment resource.
type conditionSupport struct {
	Version   int      `json:"version"`
	Supported []string `json:"supported"`
	Planned   []string `json:"planned"`
}

type iamCatalogResponse struct {
	Abilities  []abilityInfo      `json:"abilities"`
	Resources  []resourceKindInfo `json:"resource_kinds"`
	Conditions conditionSupport   `json:"conditions"`
}

func iamCatalog() iamCatalogResponse {
	return iamCatalogResponse{
		Abilities: abilityCatalog,
		Resources: []resourceKindInfo{
			{"all", "*"},
			{resourceKindApp, resourcePrefixApp + "NAME"},
			{resourceKindDatabase, resourcePrefixDatabase + "NAME"},
			{"environment", resourcePrefixEnvironment + "ID"},
			{"environment_kind", resourcePrefixEnvironmentKind + "KIND"},
		},
		Conditions: conditionSupport{Version: 1, Supported: []string{"environment_kind"}, Planned: []string{"source_ip", "time_window", "mfa"}},
	}
}

// handleIAMCatalog handles GET /api/v1/iam/catalog.
func (rt *Router) handleIAMCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, iamCatalog())
}
