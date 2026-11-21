package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

func isOracleAccount(a store.CloudAccount) bool { return a.Provider == cloudconn.ProviderOracle }

// oracleAccountConfig is the non-secret half kept on the account row, so a second
// account in another region or compartment never reads the first one's settings.
func oracleAccountConfig(req createCloudAccountRequest) map[string]string {
	cfg := map[string]string{}
	for k, v := range map[string]string{
		"region": req.Region, "tenancy_ocid": req.TenancyOCID, "user_ocid": req.UserOCID, "compartment_ocid": req.CompartmentOCID,
	} {
		if v != "" {
			cfg[k] = v
		}
	}
	return cfg
}

// oracleAccountCredential resolves one account's sealed key into the shared credential.
func (rt *Router) oracleAccountCredential(ctx context.Context, a store.CloudAccount, value string) (cloudconn.OracleCredential, error) {
	if value == "" {
		v, err := rt.accountCredential(ctx, a)
		if err != nil {
			return cloudconn.OracleCredential{}, err
		}
		value = v
	}
	return cloudconn.OracleCredentialFromStored(value)
}

// oracleProber uses the public endpoints, or the plain HTTP fake tests install.
func (rt *Router) oracleProber() cloudconn.OracleProber {
	p := cloudconn.NewOracleProber()
	if rt.oracleTestBase != "" {
		base := rt.oracleTestBase
		p.IaaS = func(string) string { return base + "/20160918" }
		p.UsageAPI = func(string) string { return base + "/20200107" }
		p.Budgets = func(string) string { return base + "/20190111" }
		p.Identity = func(string) string { return base + "/20160918" }
	}
	return p
}

// oracleCheckResult runs the account's one read-only list call and maps the class to the fixed texts.
func (rt *Router) oracleCheckResult(ctx context.Context, a store.CloudAccount, value string) cloudCheckResult {
	cred, err := rt.oracleAccountCredential(ctx, a, value)
	if err != nil {
		return cloudCheckResult{Class: cloudconn.ClassConfigInvalid, Message: cloudCheckMessages[cloudconn.ClassConfigInvalid]}
	}
	res, err := rt.oracleProber().ProbeCapabilities(ctx, cred, cloudconn.OracleCapCompute)
	if err != nil || len(res) != 1 {
		return cloudCheckResult{Class: cloudconn.ClassConfigInvalid, Message: cloudCheckMessages[cloudconn.ClassConfigInvalid]}
	}
	switch {
	case res[0].OK:
		return cloudCheckResult{OK: true, Class: cloudconn.ClassOK, Message: cloudCheckMessages[cloudconn.ClassOK]}
	case res[0].Verified:
		return cloudCheckResult{Class: cloudconn.ClassAuthFailed, Message: cloudCheckMessages[cloudconn.ClassAuthFailed], Needs: res[0].Hint}
	}
	return cloudCheckResult{Class: cloudconn.ClassUnreachable, Message: cloudCheckMessages[cloudconn.ClassUnreachable]}
}

// oracleCompartmentAllowed keeps a listing inside the account's own compartment.
func oracleCompartmentAllowed(a store.CloudAccount, compartment string) bool {
	if compartment == "" || !isOracleAccount(a) {
		return true
	}
	var cfg struct {
		Compartment string `json:"compartment_ocid"`
	}
	_ = json.Unmarshal(a.Config, &cfg)
	return cfg.Compartment == compartment
}

type oraclePolicyKitRequest struct {
	GroupName   string `json:"group_name"`
	PolicyName  string `json:"policy_name,omitempty"`
	Compartment string `json:"compartment"`
	Manage      bool   `json:"manage,omitempty"`
}

// handleOraclePolicyKit returns policy text only; nothing is created at Oracle.
func (rt *Router) handleOraclePolicyKit(w http.ResponseWriter, r *http.Request) {
	var req oraclePolicyKitRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	kit, err := cloudconn.BuildOraclePolicyKit(cloudconn.OraclePolicyKitInput{
		Group: req.GroupName, PolicyName: req.PolicyName, Compartment: req.Compartment, Manage: req.Manage,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "the policy kit inputs are not valid: group_name and a compartment name or OCID are required")
		return
	}
	writeJSON(w, http.StatusOK, kit)
}

// probeOracleAccount answers the per capability result for an Oracle account.
func (rt *Router) probeOracleAccount(w http.ResponseWriter, r *http.Request, a store.CloudAccount, names []string) {
	cred, err := rt.oracleAccountCredential(r.Context(), a, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "no credential is stored for this account")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	res, err := rt.oracleProber().ProbeCapabilities(ctx, cred, names...)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown capability or the stored credential is not valid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": res})
}

// oracleAccountRegions lists subscribed regions through the account's own key.
func (rt *Router) oracleAccountRegions(ctx context.Context, a store.CloudAccount) ([]string, error) {
	cred, err := rt.oracleAccountCredential(ctx, a, "")
	if err != nil {
		return nil, err
	}
	return rt.oracleProber().Regions(ctx, cred)
}
