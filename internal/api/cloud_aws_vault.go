package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

// vaultAccountToken resolves the Vault token for a vault_aws account: its own secret reference, else a
// login on the configured Vault connection or with Levelrail's AWS identity.
func (rt *Router) vaultAccountToken(ctx context.Context, a store.CloudAccount) (string, error) {
	var cfg cloudconn.Config
	_ = json.Unmarshal(a.Config, &cfg)
	if cfg.VaultTokenRef == "" {
		return rt.vaultLoginToken(ctx, cfg)
	}
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

func (rt *Router) vaultSettingString(ctx context.Context, key string) string {
	if rt.runtimeSettings == nil {
		return ""
	}
	return strings.TrimSpace(rt.runtimeSettings.String(ctx, key))
}

// vaultAmbientKeys is the seam for Levelrail's own AWS identity; tests replace it.
var vaultAmbientKeys = func(ctx context.Context) (vault.AWSCredentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return vault.AWSCredentials{}, err
	}
	c, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return vault.AWSCredentials{}, err
	}
	return vault.AWSCredentials{AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken}, nil
}

var (
	vaultLoginOnce  sync.Once
	vaultLoginCache *vault.AWSLogin
)

func (rt *Router) vaultTunables() func(context.Context) vault.Tunables {
	return vault.TunablesFrom(rt.runtimeSettings)
}

// vaultLoginToken signs in without a stored token: AWS auth when the setting asks for it, else the
// credential of the configured Vault connection.
func (rt *Router) vaultLoginToken(ctx context.Context, cfg cloudconn.Config) (string, error) {
	if rt.vaultSettingString(ctx, runtimesettings.KeyCloudVaultAuth) == "aws" {
		vaultLoginOnce.Do(func() { vaultLoginCache = vault.NewAWSLogin(rt.vaultTunables()) })
		return vaultLoginCache.Token(ctx, vault.AWSLoginSpec{
			Address: cfg.VaultAddress, Namespace: cfg.VaultNamespace,
			Mount:    rt.vaultSettingString(ctx, runtimesettings.KeyCloudVaultAuthMount),
			Role:     rt.vaultSettingString(ctx, runtimesettings.KeyCloudVaultAuthRole),
			ServerID: rt.vaultSettingString(ctx, runtimesettings.KeyCloudVaultServerID),
		}, vaultAmbientKeys)
	}
	if rt.vault == nil || rt.vaultSecrets == nil {
		return "", errors.New("no vault connection")
	}
	vs, err := rt.vault.GetVaultSettings(ctx)
	if err != nil || !vs.Enabled {
		return "", errors.New("vault connection is not enabled")
	}
	cred, err := rt.vaultSecrets.Resolve(ctx, store.VaultSecretsKey(), store.VaultCredentialEnvKey)
	if err != nil {
		return "", errors.New("vault connection has no credential")
	}
	return vault.NewResolverWith(rt.vaultTunables()).SessionToken(ctx, vs, cred)
}

// vaultConnectionDefaults fills the account's Vault address and namespace from the configured connection.
func (rt *Router) vaultConnectionDefaults(req *createCloudAccountRequest) {
	if req.VaultAddress != "" || rt.vault == nil {
		return
	}
	if vs, err := rt.vault.GetVaultSettings(context.Background()); err == nil && vs.Enabled {
		req.VaultAddress = vs.Address
		if req.VaultNamespace == "" {
			req.VaultNamespace = vs.Namespace
		}
	}
}

// vaultProvisionCredentials leases keys and returns the sealed-shape credential JSON the AWS factory takes.
func (rt *Router) vaultProvisionCredentials(ctx context.Context, a store.CloudAccount) (string, error) {
	creds, err := rt.accountAWSCredentials(ctx, a)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(creds) //nolint:gosec // sealed-at-rest shape handed to the factory, never logged
	return string(raw), err
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

// doctorLiveVault proves the Vault path end to end: reachable, role present, keys issue and STS accepts them.
func (rt *Router) doctorLiveVault(ctx context.Context, a store.CloudAccount, token string) []doctorStep {
	role := doctorStep{ID: "aws.vault_role", Title: "Vault reachable and role present"}
	class, err := rt.awsProvider().ProbeVault(ctx, awsTarget(a, token))
	if err != nil {
		role.Status, role.Detail = doctorFail, cloudCheckMessages[class]
		role.Fix = "Check the Vault address, the token or sign-in settings (cloud.vault.*), and that the aws secrets engine role exists."
		return []doctorStep{role, {ID: "aws.vault_creds", Title: "Vault issues working keys", Status: doctorSkip, Detail: "Skipped until Vault and the role check out."}}
	}
	role.Status, role.Detail = doctorOK, "Vault answered and the aws secrets engine role exists."
	creds := doctorStep{ID: "aws.vault_creds", Title: "Vault issues working keys"}
	k, class, err := rt.awsProvider().VaultKeys(ctx, awsTarget(a, token))
	if err != nil {
		creds.Status, creds.Detail = doctorFail, cloudCheckMessages[class]
		creds.Fix = "Check the role policy at Vault, then run the doctor again."
		return []doctorStep{role, creds}
	}
	cl, err := newAWSXClient(awsx.Credentials{AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken})
	if err == nil {
		err = cl.Query(ctx, awsx.Service{Name: "sts", Prefix: "sts", SigningName: "sts"}, "us-east-1", "GetCallerIdentity", "2011-06-15", nil, nil)
	}
	if err != nil {
		creds.Status, creds.Detail = doctorFail, "Vault issued keys but AWS did not accept them."
		creds.Fix = "Check the Vault aws secrets engine root config and the role, then try again."
		return []doctorStep{role, creds}
	}
	creds.Status, creds.Detail = doctorOK, "Vault leased keys and AWS accepted them (read-only identity check)."
	return []doctorStep{role, creds}
}
