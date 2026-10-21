package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// AlertDefaults is the surface the seeded default alert routes need from internal/alerting.DB.
type AlertDefaults interface {
	ListDefaultRules(ctx context.Context) ([]alerting.Rule, error)
	SetDefaultRuleEnabled(ctx context.Context, id string, enabled bool) (*alerting.Rule, error)
	ResetDefaultRules(ctx context.Context) ([]alerting.Rule, error)
	CertExpiryEmail(ctx context.Context) (bool, error)
	SetCertExpiryEmail(ctx context.Context, address string, on bool) error
}

// WithAlertDefaults enables the default alert list and toggle routes.
func WithAlertDefaults(a AlertDefaults) Option {
	return func(rt *Router) { rt.alertDefaults = a }
}

type defaultAlertResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Enabled  bool   `json:"enabled"`
	Firing   bool   `json:"firing"`
	// LeadTime is the cert_expiry warning window, empty for other kinds.
	LeadTime string `json:"lead_time,omitempty"`
}

func toDefaultAlertResource(r alerting.Rule) defaultAlertResource {
	out := defaultAlertResource{ID: r.ID, Name: r.Name, Kind: string(r.Kind), Severity: r.Severity, Enabled: r.Enabled, Firing: r.Firing}
	if r.Kind == alerting.KindCertExpiry && r.ForDuration > 0 {
		out.LeadTime = (r.ForDuration / time.Second * time.Second).String()
	}
	return out
}

func (rt *Router) handleListDefaultAlerts(w http.ResponseWriter, r *http.Request) {
	if rt.alertDefaults == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}
	rules, err := rt.alertDefaults.ListDefaultRules(r.Context())
	if err != nil {
		rt.internalError(w, "api: list default alerts failed", err)
		return
	}
	out := make([]defaultAlertResource, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toDefaultAlertResource(rule))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleSetDefaultAlertEnabled(w http.ResponseWriter, r *http.Request) {
	if rt.alertDefaults == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "body must be {\"enabled\": true|false}")
		return
	}
	rule, err := rt.alertDefaults.SetDefaultRuleEnabled(r.Context(), r.PathValue("id"), *req.Enabled)
	if errors.Is(err, alerting.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "default alert not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set default alert enabled failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toDefaultAlertResource(*rule))
}

func (rt *Router) handleResetDefaultAlerts(w http.ResponseWriter, r *http.Request) {
	if rt.alertDefaults == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}
	rules, err := rt.alertDefaults.ResetDefaultRules(r.Context())
	if err != nil {
		rt.internalError(w, "api: reset default alerts failed", err)
		return
	}
	out := make([]defaultAlertResource, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toDefaultAlertResource(rule))
	}
	writeJSON(w, http.StatusOK, out)
}
