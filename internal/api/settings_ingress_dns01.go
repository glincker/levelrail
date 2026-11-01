package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// SetDNS01ProviderCheck lets the dns01 setting refuse enabling while no DNS provider is connected.
func (rt *Router) SetDNS01ProviderCheck(fn func(context.Context) bool) { rt.dns01ProviderCheck = fn }

// dns01Resource is the wire shape of /api/v1/settings/ingress/dns01; source is stored or default.
type dns01Resource struct {
	Enabled           bool   `json:"enabled"`
	Source            string `json:"source"`
	ProviderConnected bool   `json:"provider_connected"`
}

type dns01UpdateRequest struct {
	Enabled *bool `json:"enabled"`
}

func (rt *Router) dns01State(ctx context.Context) dns01Resource {
	src := "default"
	if rt.runtimeSettings.Source(ctx, runtimesettings.KeyIngressDNS01AllHosts) == runtimesettings.SourceSaved {
		src = "stored"
	}
	return dns01Resource{
		Enabled:           rt.runtimeSettings.Bool(ctx, runtimesettings.KeyIngressDNS01AllHosts),
		Source:            src,
		ProviderConnected: rt.dns01ProviderCheck != nil && rt.dns01ProviderCheck(ctx),
	}
}

func (rt *Router) handleGetIngressDNS01(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	writeJSON(w, http.StatusOK, rt.dns01State(r.Context()))
}

func (rt *Router) handlePutIngressDNS01(w http.ResponseWriter, r *http.Request) {
	if !rt.runtimeSettingsOr501(w) {
		return
	}
	var req dns01UpdateRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "body must be {\"enabled\": true|false}")
		return
	}
	ctx := r.Context()
	if *req.Enabled && (rt.dns01ProviderCheck == nil || !rt.dns01ProviderCheck(ctx)) {
		writeError(w, http.StatusBadRequest, "connect a DNS provider (Cloudflare, Route53 or Google Cloud DNS) before enabling DNS-01 for every host")
		return
	}
	before, err := rt.runtimeSettings.Get(ctx, runtimesettings.KeyIngressDNS01AllHosts)
	if err != nil {
		rt.writeSettingError(w, "api: get dns01 setting", err)
		return
	}
	raw, _ := json.Marshal(*req.Enabled)
	after, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyIngressDNS01AllHosts, raw, rt.eventActor(r))
	if err != nil {
		rt.writeSettingError(w, "api: set dns01 setting", err)
		return
	}
	rt.recordSettingChange(r, before, after)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.dns01State(ctx))
}
