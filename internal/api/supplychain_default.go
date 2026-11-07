package api

import (
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/supplychain"
)

// supplyChainDefaultResource is the instance-wide scan policy apps inherit when they have none of their own.
type supplyChainDefaultResource struct {
	ScanEnabled   bool   `json:"scan_enabled"`
	ScanGate      string `json:"scan_gate"`
	ServerEnabled bool   `json:"server_enabled"`
}

func (rt *Router) toSupplyChainDefault(p supplychain.DefaultPolicy) supplyChainDefaultResource {
	gate := string(p.Gate)
	if gate == "" {
		gate = string(supplychain.GateOff)
	}
	return supplyChainDefaultResource{ScanEnabled: p.Enabled, ScanGate: gate, ServerEnabled: rt.supplyChain.Config().Enabled}
}

// handleGetSupplyChainDefault handles GET /api/v1/settings/supply-chain.
func (rt *Router) handleGetSupplyChainDefault(w http.ResponseWriter, r *http.Request) {
	if rt.supplyChain == nil {
		writeError(w, http.StatusNotImplemented, "supply chain visibility is not configured on this control plane")
		return
	}
	p, err := rt.supplyChain.DefaultPolicy(r.Context())
	if err != nil {
		rt.internalError(w, "api: load supply chain default policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toSupplyChainDefault(p))
}

// handlePutSupplyChainDefault handles PUT /api/v1/settings/supply-chain.
func (rt *Router) handlePutSupplyChainDefault(w http.ResponseWriter, r *http.Request) {
	if rt.supplyChain == nil {
		writeError(w, http.StatusNotImplemented, "supply chain visibility is not configured on this control plane")
		return
	}
	var req supplyChainSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, supplyChainBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := rt.supplyChain.SaveDefaultPolicy(r.Context(), supplychain.SettingsPatch{Enabled: req.ScanEnabled, Gate: req.ScanGate}, rt.actorForRequest(r))
	if err != nil {
		rt.writeSupplyChainError(w, err, "api: save supply chain default policy failed", "")
		return
	}
	writeJSON(w, http.StatusOK, rt.toSupplyChainDefault(p))
}
