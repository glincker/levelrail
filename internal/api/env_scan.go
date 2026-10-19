package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy/secretscan"
)

const envScanBodyLimit = 1 << 20

type envScanFinding struct {
	Var         string `json:"var"`
	RuleID      string `json:"rule_id"`
	Description string `json:"description"`
	Fingerprint string `json:"fingerprint"`
	Allowed     bool   `json:"allowed,omitempty"`
}

type envScanResponse struct {
	Findings  []envScanFinding `json:"findings"`
	Truncated bool             `json:"truncated"`
}

// handleScanEnvValues handles POST /api/v1/apps/{name}/env/scan: it checks
// pasted or about-to-be-pushed values for credentials and never echoes a value back.
func (rt *Router) handleScanEnvValues(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	var req struct {
		Vars map[string]string `json:"vars"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, envScanBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	res, err := rt.secretScan.ScanEnvNow(r.Context(), app, req.Vars)
	if err != nil {
		rt.writeSecretScanError(w, err, "api: scan pasted env failed", app)
		return
	}
	out := envScanResponse{Findings: make([]envScanFinding, 0, len(res.Findings)), Truncated: res.Truncated}
	for _, f := range res.Findings {
		out.Findings = append(out.Findings, envScanFinding{Var: f.Var, RuleID: f.RuleID, Description: secretscan.RuleDescription(f.RuleID), Fingerprint: f.Fingerprint, Allowed: f.Allowed})
	}
	rt.logger.Debug("api: env values scanned", slog.String("name", app), slog.Int("findings", len(out.Findings)))
	writeJSON(w, http.StatusOK, out)
}
