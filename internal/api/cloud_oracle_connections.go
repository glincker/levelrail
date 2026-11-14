package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
)

// oracleSealedNamespace cannot collide with an app's secrets: app names never start with an underscore.
const oracleSealedNamespace = "_cloud-oracle"

// oracleConnectionRequest is the only request type that carries an OCI private key; it is never logged or echoed.
type oracleConnectionRequest struct {
	Name            string `json:"name"`
	TenancyOCID     string `json:"tenancy_ocid"`
	Region          string `json:"region"`
	CompartmentOCID string `json:"compartment_ocid"`
	UserOCID        string `json:"user_ocid"`
	Fingerprint     string `json:"fingerprint"`
	PrivateKeyPEM   string `json:"private_key_pem"`
}

// oracleConnectionResponse names the connection; it never carries the user, the key or the sealed reference.
type oracleConnectionResponse struct {
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	AuthMethod        string `json:"auth_method"`
	Region            string `json:"region"`
	CompartmentOCID   string `json:"compartment_ocid"`
	FingerprintPrefix string `json:"fingerprint_prefix"`
}

func fingerprintPrefix(fp string) string {
	if len(fp) > 11 {
		return fp[:11]
	}
	return fp
}

func (rt *Router) handleCreateOracleConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// The server cannot see the client's transport past a proxy, so it trusts TLS, a trusted proxy's https or loopback only.
	if !requestIsHTTPS(r) && !peerIsLoopback(r) {
		writeCodedError(w, http.StatusForbidden, "tls_required", "uploading a cloud credential needs https unless the client is on this machine")
		return
	}
	svc, ok := rt.cloudConnections(w, r)
	if !ok {
		return
	}
	if rt.composeSecrets == nil {
		writeError(w, http.StatusNotImplemented, "sealed secret storage is not configured on this control plane")
		return
	}
	var req oracleConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pem := []byte(req.PrivateKeyPEM)
	defer clear(pem)
	req.PrivateKeyPEM = ""
	cred := cloudconn.OracleCredential{TenancyOCID: req.TenancyOCID, CompartmentOCID: req.CompartmentOCID, Region: req.Region,
		Secret: cloudconn.NewOracleSecret(req.UserOCID, req.Fingerprint, pem)}
	if err := cloudconn.ValidateOracle(cred); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := svc.Get(r.Context(), req.Name); err == nil {
		writeCodedError(w, http.StatusConflict, "connection_exists", "a cloud connection with this name already exists")
		return
	} else if !errors.Is(err, cloudconn.ErrNotFound) {
		rt.writeCloudConnectionError(w, "api: look up oracle connection failed", err)
		return
	}
	// A random suffix keeps a racing create from overwriting another connection's sealed bundle.
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		rt.internalError(w, "api: oracle seal key failed", err)
		return
	}
	ref, err := cloudconn.SealOracleSecret(r.Context(), rt.composeSecrets, oracleSealedNamespace, req.Name+"."+hex.EncodeToString(suffix), cred.Secret)
	if err != nil {
		rt.writeCloudConnectionError(w, "api: seal oracle credential failed", err)
		return
	}
	_, actor, _, _ := rt.callerPrincipal(r)
	c, err := svc.Create(r.Context(), actor, req.Name, cloudconn.Input{
		Provider: cloudconn.ProviderOracle, AuthMethod: cloudconn.AuthStatic, CredentialRef: ref,
		Config: cloudconn.Config{Region: req.Region, AccountID: req.TenancyOCID, Compartment: req.CompartmentOCID},
	})
	if errors.Is(err, cloudconn.ErrConflict) {
		writeCodedError(w, http.StatusConflict, "connection_exists", "a cloud connection with this name already exists")
		return
	}
	if err != nil {
		rt.writeCloudConnectionError(w, "api: create oracle connection failed", err)
		return
	}
	rt.auditCloudConnection(r, auditCloudConnCreated, c, "static oracle key sealed, fingerprint "+fingerprintPrefix(req.Fingerprint))
	writeJSON(w, http.StatusCreated, oracleConnectionResponse{
		Name: c.Name, Provider: c.Provider, AuthMethod: c.AuthMethod, Region: c.Config.Region,
		CompartmentOCID: c.Config.Compartment, FingerprintPrefix: fingerprintPrefix(req.Fingerprint),
	})
}
