package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

// vaultAccountToken resolves the Vault token for a vault_aws account from its secret reference.
func (rt *Router) vaultAccountToken(ctx context.Context, a store.CloudAccount) (string, error) {
	var cfg cloudconn.Config
	_ = json.Unmarshal(a.Config, &cfg)
	ref, err := secretsource.ParseRef(cfg.VaultTokenRef)
	if err != nil {
		return "", err
	}
	sc, ok := rt.secretConns.(*secretconn.Service)
	if !ok || sc == nil || sc.Sources == nil {
		return "", errors.New("no secret sources")
	}
	v, err := sc.Sources.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	return string(v.Reveal()), nil
}

// revokeVaultAccount ends the account's Vault leases; a failure is logged and never blocks the delete or disable.
func (rt *Router) revokeVaultAccount(ctx context.Context, a store.CloudAccount) {
	if !isAWSVaultAccount(a) {
		return
	}
	tok, _ := rt.vaultAccountToken(ctx, a)
	if err := rt.awsProvider().RevokeVault(ctx, awsTarget(a, tok)); err != nil {
		rt.logger.Warn("api: revoke vault aws lease failed", slog.String("account", a.ID))
	}
}

type awsVaultKitRequest struct {
	AccountID    string   `json:"account_id"`
	RoleName     string   `json:"role_name,omitempty"`
	Region       string   `json:"region,omitempty"`
	VaultMount   string   `json:"vault_mount,omitempty"`
	VaultRole    string   `json:"vault_role,omitempty"`
	Namespace    string   `json:"vault_namespace,omitempty"`
	PrincipalARN string   `json:"vault_principal_arn"`
	Capabilities []string `json:"capabilities,omitempty"`
	Regions      []string `json:"regions,omitempty"`
	HostedZones  []string `json:"hosted_zone_ids,omitempty"`
}

func (rt *Router) handleAWSVaultKit(w http.ResponseWriter, r *http.Request) {
	var req awsVaultKitRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := requireZonesForZoneTiers(req.Capabilities, req.HostedZones); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kit, err := cloudconn.BuildVaultKit(cloudconn.VaultKitInput{AccountID: req.AccountID, RoleName: req.RoleName, Region: req.Region, Mount: req.VaultMount,
		VaultRole: req.VaultRole, Namespace: req.Namespace, PrincipalARN: req.PrincipalARN, Capabilities: req.Capabilities, Regions: req.Regions, HostedZones: req.HostedZones, NamePrefix: rt.awsNamePrefix()})
	if err != nil {
		writeError(w, http.StatusBadRequest, "the vault kit inputs are not valid")
		return
	}
	writeJSON(w, http.StatusOK, kit)
}
