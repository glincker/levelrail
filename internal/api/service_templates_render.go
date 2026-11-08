package api

import (
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const maskedValue = "********"

type templatePlanEnv struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Source string `json:"source"`
	Secret bool   `json:"secret,omitempty"`
}

// templateInstallPlan is the dry-run answer: what an install would deploy,
// with nothing stored and every secret value masked.
type templateInstallPlan struct {
	DryRun  bool              `json:"dry_run"`
	Name    string            `json:"name"`
	Missing []string          `json:"missing,omitempty"`
	Env     []templatePlanEnv `json:"env"`
	Compose string            `json:"compose"`
}

// buildTemplateInstallPlan renders the manifest tolerantly so required
// values the operator forgot are listed together instead of one by one.
func buildTemplateInstallPlan(m *catalog.Manifest, name string, req templateInstallRequest) (templateInstallPlan, error) {
	plan := templateInstallPlan{DryRun: true, Name: name, Env: []templatePlanEnv{}}
	var secretValues []string
	for _, p := range m.Env {
		entry := templatePlanEnv{Key: p.Key, Secret: p.Secret}
		supplied := req.Env[p.Key]
		switch {
		case supplied != "":
			entry.Source = "supplied"
			entry.Value = supplied
		case p.Generate != "":
			entry.Source = "generated"
		case p.Default != "":
			entry.Source = "default"
			entry.Value = p.Default
		case p.Required:
			entry.Source = "missing"
			plan.Missing = append(plan.Missing, p.Key)
		default:
			entry.Source = "empty"
		}
		if p.Secret || p.Generate != "" {
			if entry.Value != "" {
				secretValues = append(secretValues, entry.Value)
				entry.Value = maskedValue
			}
		}
		plan.Env = append(plan.Env, entry)
	}
	body, err := m.Render(catalog.InstallInput{Env: req.Env, Domains: req.Domains}, true)
	if err != nil {
		return plan, err
	}
	for _, v := range secretValues {
		body = strings.ReplaceAll(body, v, maskedValue)
	}
	plan.Compose = body
	return plan, nil
}

func (rt *Router) writeTemplateInstallPlan(w http.ResponseWriter, tpl catalog.Template, req templateInstallRequest) {
	name := req.Name
	if name == "" {
		name = tpl.ID
	}
	if !installAppName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "name must start with a letter and use only lowercase letters, digits and dashes (40 characters at most)")
		return
	}
	plan, err := buildTemplateInstallPlan(tpl.Manifest, name, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
