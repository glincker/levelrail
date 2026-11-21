package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

// methodOCISession is the Oracle browser login method.
const methodOCISession = cloudconn.AuthOCISession

const oracleSessionExpKey = "session_expires_at"

func isOracleSessionAccount(a store.CloudAccount) bool {
	return isOracleAccount(a) && a.AuthMethod == methodOCISession
}

// parseOCIProfile reads tenancy and region from pasted ~/.oci/config text. Only those two
// keys are kept, so a pasted file with a key_file path or fingerprint leaks nothing.
func parseOCIProfile(text string) (tenancy, region string) {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "tenancy":
			tenancy = strings.TrimSpace(v)
		case "region":
			region = strings.TrimSpace(v)
		}
	}
	return tenancy, region
}

// oracleSessionValue validates the session fields offline and returns the sealed JSON and exp.
func oracleSessionValue(req createCloudAccountRequest) (value string, exp time.Time, msg string) {
	if req.ProfileConfig != "" {
		t, rg := parseOCIProfile(req.ProfileConfig)
		if req.TenancyOCID == "" {
			req.TenancyOCID = t
		}
		if req.Region == "" {
			req.Region = rg
		}
	}
	if req.SessionToken == "" || req.PrivateKeyPEM == "" || req.TenancyOCID == "" || req.Region == "" || req.CompartmentOCID == "" {
		return "", time.Time{}, "tenancy_ocid, region, compartment_ocid, session_token and private_key_pem are required for oci_session"
	}
	cred := cloudconn.OracleCredential{
		TenancyOCID: req.TenancyOCID, CompartmentOCID: req.CompartmentOCID, Region: req.Region,
		Secret: cloudconn.NewOracleSecret("", "", []byte(req.PrivateKeyPEM)), SessionToken: strings.TrimSpace(req.SessionToken),
	}
	exp, err := cloudconn.ValidateOracleSession(cred, time.Now())
	if err != nil {
		return "", exp, "the Oracle session is not valid: " + sanitizeProviderErr(err)
	}
	enc, err := json.Marshal(cloudconn.OracleStored{ //nolint:gosec // sealed by nodeProviderSecrets.SetValue, never logged or returned
		TenancyOCID: req.TenancyOCID, PrivateKeyPEM: req.PrivateKeyPEM, Region: req.Region,
		CompartmentOCID: req.CompartmentOCID, SessionToken: cred.SessionToken,
	})
	if err != nil {
		return "", exp, "internal error"
	}
	return string(enc), exp, ""
}

// sessionExpiry reads the non-secret exp kept on the account row.
func sessionExpiry(a store.CloudAccount) (time.Time, bool) {
	var cfg map[string]any
	if json.Unmarshal(a.Config, &cfg) != nil {
		return time.Time{}, false
	}
	s, _ := cfg[oracleSessionExpKey].(string)
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// oracleSessionNeedsRelogin is true for an oci_session account past or near exp.
func oracleSessionNeedsRelogin(a store.CloudAccount) bool {
	if !isOracleSessionAccount(a) {
		return false
	}
	exp, ok := sessionExpiry(a)
	return !ok || cloudconn.OracleSessionNeedsRelogin(exp, time.Now())
}

func needsReloginResult() cloudCheckResult {
	return cloudCheckResult{Class: cloudconn.ClassNeedsRelogin, Message: cloudCheckMessages[cloudconn.ClassNeedsRelogin],
		Needs: cloudconn.OracleReloginCommand("")}
}

// replaceOracleSession reseals a fresh token and key for an existing oci_session account.
func (rt *Router) replaceOracleSession(ctx context.Context, a store.CloudAccount, token, keyPEM string) (store.CloudAccount, string) {
	var cfg map[string]any
	_ = json.Unmarshal(a.Config, &cfg)
	str := func(k string) string { v, _ := cfg[k].(string); return v }
	req := createCloudAccountRequest{ProfileConfig: ""}
	req.TenancyOCID, req.Region, req.CompartmentOCID = str("tenancy_ocid"), str("region"), str("compartment_ocid")
	req.SessionToken, req.PrivateKeyPEM = token, keyPEM
	value, exp, msg := oracleSessionValue(req)
	if msg != "" {
		return a, msg
	}
	if a.CredentialRef == "" {
		a.CredentialRef = cloudAccountSecretPref + a.ID
	}
	if err := rt.nodeProviderSecrets.SetValue(ctx, a.CredentialRef, store.NodeProviderTokenEnvKey, value); err != nil {
		return a, "storing the session failed"
	}
	cfg[oracleSessionExpKey] = exp.Format(time.RFC3339)
	a.Config, _ = json.Marshal(cfg)
	return a, ""
}

type oracleSessionImportRequest struct {
	ProfileConfig   string `json:"profile_config,omitempty"`
	SessionToken    string `json:"session_token"`
	PrivateKeyPEM   string `json:"private_key_pem"`
	TenancyOCID     string `json:"tenancy_ocid,omitempty"`
	Region          string `json:"region,omitempty"`
	CompartmentOCID string `json:"compartment_ocid,omitempty"`
}

// handleOracleSessionImport validates pasted security_token_file and key_file contents without
// any outbound call. The response never echoes the token or the key.
func (rt *Router) handleOracleSessionImport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req oracleSessionImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, rg := parseOCIProfile(req.ProfileConfig)
	if req.TenancyOCID == "" {
		req.TenancyOCID = t
	}
	if req.Region == "" {
		req.Region = rg
	}
	if req.CompartmentOCID == "" {
		// A tenancy is accepted here only to get through shape checks; the caller supplies the real compartment on create.
		req.CompartmentOCID = strings.Replace(req.TenancyOCID, "ocid1.tenancy.", "ocid1.compartment.", 1)
	}
	creq := createCloudAccountRequest{ProfileConfig: ""}
	creq.TenancyOCID, creq.Region, creq.CompartmentOCID = req.TenancyOCID, req.Region, req.CompartmentOCID
	creq.SessionToken, creq.PrivateKeyPEM = req.SessionToken, req.PrivateKeyPEM
	_, exp, msg := oracleSessionValue(creq)
	if msg != "" && exp.IsZero() {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	expired := msg != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"tenancy_ocid": req.TenancyOCID, "region": req.Region, "expires_at": exp,
		"needs_relogin":   expired || cloudconn.OracleSessionNeedsRelogin(exp, time.Now()),
		"relogin_command": cloudconn.OracleReloginCommand(""),
	})
}
