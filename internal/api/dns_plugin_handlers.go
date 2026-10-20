package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

// checkDNSProviderPlugin returns why ov's plugin choice is unusable, or "".
func (rt *Router) checkDNSProviderPlugin(ctx context.Context, ov domainAutomationOverride) string {
	name := ov.dnsPluginName()
	if name == "" {
		return ""
	}
	if rt.plugins == nil {
		return "the plugin runtime is not available on this control plane"
	}
	rec, err := rt.plugins.Get(ctx, name)
	if err != nil || rec.Kind != plugin.KindDNSProvider {
		return "dns_provider_plugin must name an installed dns_provider plugin"
	}
	return ""
}

type dnsProviderPluginResource struct {
	Plugin string `json:"plugin"`
	// Provider is the DNS provider whose credential the plugin is handed.
	Provider string `json:"provider"`
}

type setDNSProviderPluginRequest struct {
	Plugin string `json:"plugin"`
}

// handleGetDNSProviderPlugin serves GET /api/v1/settings/dns-provider-plugin.
func (rt *Router) handleGetDNSProviderPlugin(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, dnsProviderPluginResource{Plugin: rt.dnsProviderPluginName(r.Context()), Provider: rt.activeDNSProvider(r.Context())})
}

// handleSetDNSProviderPlugin serves PUT /api/v1/settings/dns-provider-plugin.
// An empty plugin returns DNS writes to the built-in client.
func (rt *Router) handleSetDNSProviderPlugin(w http.ResponseWriter, r *http.Request) {
	st := rt.automationStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "domain automation is not supported by this store")
		return
	}
	var req setDNSProviderPluginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, domainAutomationMaxBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ov := rt.storedAutomation(r.Context())
	ov.DNSProviderPlugin = &req.Plugin
	if msg := ov.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := rt.checkDNSProviderPlugin(r.Context(), ov); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	raw, err := json.Marshal(ov)
	if err != nil {
		rt.internalError(w, "api: encode domain automation", err)
		return
	}
	if err := st.SetDomainAutomationRaw(r.Context(), string(raw)); err != nil {
		rt.internalError(w, "api: save dns provider plugin", err)
		return
	}
	writeJSON(w, http.StatusOK, dnsProviderPluginResource{Plugin: req.Plugin, Provider: rt.activeDNSProvider(r.Context())})
}

// handleTestPlugin serves POST /api/v1/plugins/{name}/test: a read-only
// round trip that proves a dns_provider plugin is reachable and its
// credential works.
func (rt *Router) handleTestPlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.pluginManager(w)
	if !ok {
		return
	}
	rec, err := m.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writePluginError(w, "test", err)
		return
	}
	if rec.Kind != plugin.KindDNSProvider {
		writeError(w, http.StatusBadRequest, "only dns_provider plugins have a connection test")
		return
	}
	writeJSON(w, http.StatusOK, rt.testDNSPlugin(r.Context(), rec.Name))
}
