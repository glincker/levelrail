package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Alert template profiles: the app shapes a one-click rule set exists for.
const (
	AlertProfileWeb      = "web"
	AlertProfileWorker   = "worker"
	AlertProfileDatabase = "database"
)

type alertTemplateRule struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type alertTemplateProfile struct {
	Profile string              `json:"profile"`
	Rules   []alertTemplateRule `json:"rules"`
}

type applyAlertTemplateRequest struct {
	Profile   string `json:"profile"`
	ChannelID string `json:"channel_id"`
}

type applyAlertTemplateResponse struct {
	Profile string         `json:"profile"`
	Created []ruleResource `json:"created"`
	Skipped []string       `json:"skipped"`
}

func crashloopTemplate() ruleResource {
	return ruleResource{Name: "Crash loop", Kind: string(alerting.KindCrashloop), RestartCountThreshold: 3, RestartWindow: "10m", Severity: alerting.SeverityCritical}
}

func cpuTemplate(threshold float64) ruleResource {
	return ruleResource{Name: "High CPU", Kind: string(alerting.KindThreshold), Metric: "cpu_percent", Comparator: string(alerting.GreaterThan), Threshold: threshold, ForDuration: "10m", Severity: alerting.SeverityWarning}
}

// alertTemplateRules returns the rule set for a profile, or false for an unknown one.
func alertTemplateRules(profile string) ([]ruleResource, bool) {
	switch profile {
	case AlertProfileWeb:
		return []ruleResource{
			crashloopTemplate(),
			{Name: "High 5xx error rate", Kind: string(alerting.KindThreshold), Metric: alerting.MetricRequestErrorRate5xxPct, Comparator: string(alerting.GreaterThan), Threshold: 5, ForDuration: "5m", Severity: alerting.SeverityWarning},
			{Name: "Slow responses (p95)", Kind: string(alerting.KindThreshold), Metric: alerting.MetricRequestP95Ms, Comparator: string(alerting.GreaterThan), Threshold: 1000, ForDuration: "5m", Severity: alerting.SeverityWarning},
			cpuTemplate(90),
			{Name: "Availability SLO 99.9%", Kind: string(alerting.KindSLOBurn), SLO: &alerting.SLOConfig{Objective: alerting.SLOAvailability, Target: 99.9}, Severity: alerting.SeverityCritical},
		}, true
	case AlertProfileWorker:
		return []ruleResource{crashloopTemplate(), cpuTemplate(90)}, true
	case AlertProfileDatabase:
		return []ruleResource{crashloopTemplate(), cpuTemplate(85)}, true
	}
	return nil, false
}

var alertTemplateDescriptions = map[string]string{
	string(alerting.KindCrashloop): "Restarts repeatedly in a short window",
	string(alerting.KindThreshold): "A metric stays past its limit",
	string(alerting.KindSLOBurn):   "Error budget burns faster than sustainable",
}

// handleListAlertTemplates handles GET /api/v1/alert-templates.
func (rt *Router) handleListAlertTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]alertTemplateProfile, 0, 3)
	for _, p := range []string{AlertProfileWeb, AlertProfileWorker, AlertProfileDatabase} {
		rules, _ := alertTemplateRules(p)
		prof := alertTemplateProfile{Profile: p}
		for _, r := range rules {
			prof.Rules = append(prof.Rules, alertTemplateRule{Name: r.Name, Kind: r.Kind, Severity: r.Severity, Description: alertTemplateDescriptions[r.Kind]})
		}
		out = append(out, prof)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleApplyAlertTemplate handles POST /api/v1/apps/{name}/alerts/apply-template.
// Rules whose name already exists on the app are skipped, so applying twice is safe.
func (rt *Router) handleApplyAlertTemplate(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "apply alert template: load app", err)
		return
	}

	var req applyAlertTemplateRequest
	if !decodeReq(w, r, &req) {
		return
	}
	templates, ok := alertTemplateRules(req.Profile)
	if !ok {
		writeError(w, http.StatusBadRequest, "profile must be web, worker or database")
		return
	}

	existing, err := rt.alertRules.ListRulesForResource(r.Context(), resourceIDForApp(name))
	if err != nil {
		rt.internalError(w, "apply alert template: list rules", err)
		return
	}
	have := make(map[string]bool, len(existing))
	for _, e := range existing {
		have[e.Name] = true
	}

	resp := applyAlertTemplateResponse{Profile: req.Profile, Created: []ruleResource{}, Skipped: []string{}}
	for _, t := range templates {
		if have[t.Name] {
			resp.Skipped = append(resp.Skipped, t.Name)
			continue
		}
		t.ResourceID = resourceIDForApp(name)
		t.ChannelID = req.ChannelID
		t.Enabled = true
		id, err := alerting.NewRuleID()
		if err != nil {
			rt.internalError(w, "apply alert template: generate id", err)
			return
		}
		rule, err := t.toRule(id)
		if err != nil {
			rt.internalError(w, "api: alert template is invalid", err, slog.String("rule", t.Name))
			return
		}
		if !rt.validateAlertRuleReferences(w, r, name, "apply alert template", rule) {
			return
		}
		if err := rt.alertRules.SaveRule(r.Context(), rule); err != nil {
			rt.internalError(w, "apply alert template: save rule", err)
			return
		}
		saved, err := rt.alertRules.GetRule(r.Context(), id)
		if err != nil {
			rt.internalError(w, "apply alert template: reload rule", err)
			return
		}
		res := toRuleResource(*saved)
		res.Warnings = ruleWarnings(*saved)
		resp.Created = append(resp.Created, res)
	}
	writeJSON(w, http.StatusCreated, resp)
}
