package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type probeRegistryCredentialRequest struct {
	RegistryHost string `json:"registry_host"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

// handleProbeRegistryCredential handles POST
// /api/v1/registry-credentials/test: authenticates a candidate credential
// against its registry and stores nothing, so the form can verify before
// saving. The password is never echoed or logged.
func (rt *Router) handleProbeRegistryCredential(w http.ResponseWriter, r *http.Request) {
	if rt.registryAuthTester == nil {
		writeError(w, http.StatusNotImplemented, "registry credential testing is not configured on this control plane")
		return
	}
	var req probeRegistryCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.RegistryHost) == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "registry_host, username and password are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	if err := rt.registryAuthTester.TestRegistryAuth(ctx, req.RegistryHost, req.Username, req.Password); err != nil {
		writeJSON(w, http.StatusOK, nodeProviderTestResult{OK: false, Message: registryAuthTestErrorMessage(err), TestedAt: time.Now().UTC()})
		return
	}
	writeJSON(w, http.StatusOK, nodeProviderTestResult{OK: true, Message: "Authenticated: the registry accepted the credential.", TestedAt: time.Now().UTC()})
}
