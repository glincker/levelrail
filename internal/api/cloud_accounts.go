package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	cloudAccountSecretPref = "cloud-account/"
	cloudAccountMethodRole = cloudconn.AuthRoleAssumption
)

var cloudAccountIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

var awsGuardPolicyRe = regexp.MustCompile(`^arn:aws[a-z-]*:iam::[0-9]{12}:policy/[A-Za-z0-9+=,.@_/-]{1,512}$`)

// cloudAccountStore is the store surface the account and grant routes need; *store.DB satisfies it.
type cloudAccountStore interface {
	cloudAuthzStore
	CreateCloudAccount(ctx context.Context, a store.CloudAccount) error
	ListCloudAccounts(ctx context.Context, provider string) ([]store.CloudAccount, error)
	UpdateCloudAccount(ctx context.Context, a store.CloudAccount) error
	MarkCloudAccountVerified(ctx context.Context, id string, at time.Time) error
	DeleteCloudAccount(ctx context.Context, id string) error
	ListCloudGrants(ctx context.Context, accountID string) ([]store.CloudGrant, error)
	UpsertCloudGrant(ctx context.Context, g store.CloudGrant) error
	DeleteCloudGrant(ctx context.Context, id string) error
}

var errCloudStoreMissing = errors.New("cloud accounts are not available on this store")

func (rt *Router) cloudAcctStore() (cloudAccountStore, bool) {
	s, ok := rt.apps.(cloudAccountStore)
	return s, ok
}

// cloudAuthzRef resolves the store per call so routes can register before the store is known.
type cloudAuthzRef struct{ rt *Router }

func (c cloudAuthzRef) GetCloudAccount(ctx context.Context, id string) (store.CloudAccount, error) {
	s, ok := c.rt.cloudAcctStore()
	if !ok {
		return store.CloudAccount{}, errCloudStoreMissing
	}
	return s.GetCloudAccount(ctx, id)
}

func (c cloudAuthzRef) ListCloudGrantsForPrincipals(ctx context.Context, accountID string, p []store.CloudPrincipal) ([]store.CloudGrant, error) {
	s, ok := c.rt.cloudAcctStore()
	if !ok {
		return nil, errCloudStoreMissing
	}
	return s.ListCloudGrantsForPrincipals(ctx, accountID, p)
}

func (c cloudAuthzRef) UserTeamRoles(ctx context.Context, userID string) (map[string]string, error) {
	s, ok := c.rt.cloudAcctStore()
	if !ok {
		return nil, errCloudStoreMissing
	}
	return s.UserTeamRoles(ctx, userID)
}

func (rt *Router) cloudStoreOr501(w http.ResponseWriter) (cloudAccountStore, bool) {
	s, ok := rt.cloudAcctStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, errCloudStoreMissing.Error())
	}
	return s, ok
}

func cloudAccountFromPath(r *http.Request) cloudResource {
	return cloudAccountResource(r.PathValue("id"))
}

// cloudAccountWire is the account shape; it never carries a credential.
type cloudAccountWire struct {
	ID             string          `json:"id"`
	Provider       string          `json:"provider"`
	Label          string          `json:"label"`
	Method         string          `json:"method"`
	Config         json.RawMessage `json:"config"`
	TeamID         string          `json:"team_id,omitempty"`
	Status         string          `json:"status"`
	Linked         bool            `json:"linked"`
	LastVerifiedAt *time.Time      `json:"last_verified_at,omitempty"`
	CreatedBy      string          `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	// Session fields are set only for oci_session accounts and carry no secret.
	SessionExpiresAt *time.Time `json:"session_expires_at,omitempty"`
	NeedsRelogin     bool       `json:"needs_relogin,omitempty"`
	ReloginCommand   string     `json:"relogin_command,omitempty"`
}

func (rt *Router) toCloudAccountWire(ctx context.Context, a store.CloudAccount) cloudAccountWire {
	cfg := a.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	linked := false
	if isAWSVaultAccount(a) {
		linked = strings.Contains(string(cfg), "vault_token_ref")
	} else if rt.nodeProviderSecrets != nil {
		linked, _ = rt.nodeProviderSecrets.Exists(ctx, cloudAccountSecretRef(a), store.NodeProviderTokenEnvKey)
	}
	w := cloudAccountWire{ID: a.ID, Provider: a.Provider, Label: a.DisplayName, Method: a.AuthMethod, Config: cfg, TeamID: a.TeamID,
		Status: a.Status, Linked: linked, LastVerifiedAt: a.LastVerifiedAt, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
	if isOracleSessionAccount(a) {
		if exp, ok := sessionExpiry(a); ok {
			w.SessionExpiresAt = &exp
		}
		if w.NeedsRelogin = oracleSessionNeedsRelogin(a); w.NeedsRelogin {
			w.ReloginCommand = cloudconn.OracleReloginCommand("")
		}
	}
	return w
}

// cloudAccountSecretRef is the secrets service name; an account with no ref uses the legacy per provider key.
func cloudAccountSecretRef(a store.CloudAccount) string {
	if a.CredentialRef != "" {
		return a.CredentialRef
	}
	return store.NodeProviderSecretsKey(a.Provider)
}

// cloudAccountVisible filters accounts to what the caller may view: root sees all, others need a grant or team admin.
func (rt *Router) cloudAccountVisible(r *http.Request, st cloudAccountStore, accts []store.CloudAccount) ([]store.CloudAccount, error) {
	ptype, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, err
	}
	root := hasAbility(abilities, AbilityRoot)
	var caller cloudCaller
	var grants []store.CloudGrant
	if !root {
		if caller, err = rt.cloudCallerFor(r.Context(), st, ptype, pid); err != nil {
			return nil, err
		}
		if grants, err = st.ListCloudGrantsForPrincipals(r.Context(), "", caller.principals); err != nil {
			return nil, err
		}
	}
	out := []store.CloudAccount{}
	for _, a := range accts {
		res := cloudAccountResource(a.ID)
		denied, derr := rt.cloudDenied(r.Context(), ptype, pid, AbilityRead, res)
		if derr != nil {
			return nil, derr
		}
		d := authorizeCloud(cloudAuthzInput{Action: store.CloudActionView, Resource: res, Now: time.Now().UTC(), Root: root, Denied: denied,
			Grants: grants, OwnerTeamAdmin: a.TeamID != "" && isTeamAdminRole(caller.teamRoles[a.TeamID])})
		if d.Allowed {
			out = append(out, a)
		}
	}
	return out, nil
}

func (rt *Router) handleListCloudAccounts(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	accts, err := st.ListCloudAccounts(r.Context(), r.URL.Query().Get("provider"))
	if err != nil {
		rt.internalError(w, "api: list cloud accounts failed", err)
		return
	}
	if accts, err = rt.cloudAccountVisible(r, st, accts); err != nil {
		rt.internalError(w, "api: filter cloud accounts failed", err)
		return
	}
	out := make([]cloudAccountWire, 0, len(accts))
	for _, a := range accts {
		out = append(out, rt.toCloudAccountWire(r.Context(), a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetCloudAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.toCloudAccountWire(r.Context(), a))
}

func (rt *Router) loadCloudAccount(w http.ResponseWriter, r *http.Request) (store.CloudAccount, bool) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return store.CloudAccount{}, false
	}
	a, err := st.GetCloudAccount(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrCloudAccountNotFound) {
		writeError(w, http.StatusNotFound, "cloud account not found")
		return store.CloudAccount{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load cloud account failed", err)
		return store.CloudAccount{}, false
	}
	return a, true
}

type createCloudAccountRequest struct {
	setNodeProviderCredentialRequest
	ID        string `json:"id,omitempty"`
	TeamID    string `json:"team_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	// ExternalID is the value from the link kit, stored sealed for method role_assumption.
	ExternalID string `json:"external_id,omitempty"`
	// Vault fields apply to method vault_aws; the token is only ever a secret:// reference.
	VaultAddress   string `json:"vault_address,omitempty"`
	VaultMount     string `json:"vault_mount,omitempty"`
	VaultRole      string `json:"vault_role,omitempty"`
	VaultNamespace string `json:"vault_namespace,omitempty"`
	VaultTokenRef  string `json:"vault_token_ref,omitempty"`
	// ProfileConfig is pasted ~/.oci/config text for oci_session; only tenancy and region are read.
	ProfileConfig string `json:"profile_config,omitempty"`
}

func newCloudID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func isAWSRoleAccount(a store.CloudAccount) bool {
	return a.Provider == cloudconn.ProviderAWS && a.AuthMethod == cloudAccountMethodRole
}

func isAWSVaultAccount(a store.CloudAccount) bool {
	return a.Provider == cloudconn.ProviderAWS && a.AuthMethod == cloudconn.AuthVaultAWS
}

// isAWSBrokeredAccount covers the methods that assume a role or lease short lived keys.
func isAWSBrokeredAccount(a store.CloudAccount) bool {
	return isAWSRoleAccount(a) || isAWSVaultAccount(a)
}

func awsVaultAccountConfig(req createCloudAccountRequest) (map[string]string, string) {
	if req.Region == "" || req.VaultTokenRef == "" {
		return nil, "region, vault_address, vault_role and vault_token_ref are required for vault_aws"
	}
	cc := cloudconn.Config{Region: req.Region, AccountID: req.AccountID, VaultAddress: req.VaultAddress, VaultMount: req.VaultMount,
		VaultRole: req.VaultRole, VaultNamespace: req.VaultNamespace, VaultTokenRef: req.VaultTokenRef}
	if err := cloudconn.NewAWSProvider(cloudconn.AWSOptions{}).ValidateConfig(cc, cloudconn.AuthVaultAWS); err != nil {
		return nil, "the account settings are not valid"
	}
	cfg := map[string]string{"region": cc.Region, "vault_address": cc.VaultAddress, "vault_role": cc.VaultRole, "vault_token_ref": cc.VaultTokenRef}
	for k, v := range map[string]string{"account_id": cc.AccountID, "vault_mount": cc.VaultMount, "vault_namespace": cc.VaultNamespace} {
		if v != "" {
			cfg[k] = v
		}
	}
	return cfg, ""
}

// cloudAccountDraft validates a create body into the row and the sealed credential value.
func (rt *Router) cloudAccountDraft(req createCloudAccountRequest) (store.CloudAccount, string, string) {
	if req.Provider == nodeProviderCustom || !isKnownNodeProvider(req.Provider) {
		return store.CloudAccount{}, "", unknownNodeProviderMessage()
	}
	if len(req.Label) > nodeProviderMaxMetaLen {
		return store.CloudAccount{}, "", "label is limited to 500 characters"
	}
	cfg := map[string]string{}
	var value string
	method := req.Method
	if req.Provider == cloudconn.ProviderAWS && method == cloudconn.AuthVaultAWS {
		var msg string
		if cfg, msg = awsVaultAccountConfig(req); msg != "" {
			return store.CloudAccount{}, "", msg
		}
	} else if req.Provider == cloudconn.ProviderAWS && method == cloudAccountMethodRole {
		if req.RoleARN == "" || req.ExternalID == "" || req.Region == "" {
			return store.CloudAccount{}, "", "role_arn, external_id and region are required for role_assumption"
		}
		p := cloudconn.NewAWSProvider(cloudconn.AWSOptions{})
		if err := p.ValidateConfig(cloudconn.Config{Region: req.Region, AccountID: req.AccountID, RoleARN: req.RoleARN}, method); err != nil {
			return store.CloudAccount{}, "", "the account settings are not valid"
		}
		cfg["region"], cfg["role_arn"] = req.Region, req.RoleARN
		if req.AccountID != "" {
			cfg["account_id"] = req.AccountID
		}
		value = req.ExternalID
	} else {
		if method == "" {
			method = inferNodeProviderMethod(req.setNodeProviderCredentialRequest)
		}
		req.Method = method
		if !methodAllowed(req.Provider, method) {
			return store.CloudAccount{}, "", "method " + method + " is not supported for " + req.Provider
		}
		var msg string
		var sessionExp time.Time
		if req.Provider == cloudconn.ProviderOracle && method == methodOCISession {
			value, sessionExp, msg = oracleSessionValue(req)
		} else if req.Provider == cloudconn.ProviderOracle {
			value, msg = oracleCredentialValueOpt(req.setNodeProviderCredentialRequest, false)
		} else {
			value, msg = nodeProviderCredentialValue(req.setNodeProviderCredentialRequest)
		}
		if msg != "" {
			return store.CloudAccount{}, "", msg
		}
		if req.Region != "" {
			cfg["region"] = req.Region
		}
		if req.Provider == cloudconn.ProviderOracle {
			cfg = oracleAccountConfig(req)
			if method == methodOCISession {
				cfg[oracleSessionExpKey] = sessionExp.Format(time.RFC3339)
			}
		}
	}
	raw, _ := json.Marshal(cfg)
	id := req.ID
	if id != "" && (!cloudAccountIDRe.MatchString(id) || strings.HasSuffix(id, store.CloudAccountDefaultSuffix)) {
		return store.CloudAccount{}, "", "id must be lowercase letters, digits and hyphens, and cannot end in -default"
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = req.Provider
	}
	return store.CloudAccount{ID: id, Provider: req.Provider, DisplayName: label, AuthMethod: method, Config: raw, TeamID: req.TeamID}, value, ""
}

func (rt *Router) handleCreateCloudAccount(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloud accounts need a master key on this control plane")
		return
	}
	var req createCloudAccountRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	acct, value, msg := rt.cloudAccountDraft(req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if acct.ID == "" {
		id, err := newCloudID(acct.Provider + "-")
		if err != nil {
			rt.internalError(w, "api: mint cloud account id failed", err)
			return
		}
		acct.ID = id
	}
	if _, err := st.GetCloudAccount(r.Context(), acct.ID); err == nil {
		writeError(w, http.StatusConflict, "a cloud account with this id already exists")
		return
	}
	acct.CredentialRef = cloudAccountSecretPref + acct.ID
	acct.CreatedBy = rt.nodeProviderActor(r)
	if isAWSVaultAccount(acct) {
		tok, err := rt.vaultAccountToken(r.Context(), acct)
		if err != nil {
			writeError(w, http.StatusBadRequest, "the vault token reference could not be resolved")
			return
		}
		value = tok
	}
	if req.Verify {
		if res := rt.checkCloudAccount(r.Context(), acct, value); !res.OK {
			writeJSON(w, http.StatusUnprocessableEntity, res)
			return
		}
	}
	if isAWSVaultAccount(acct) {
		acct.CredentialRef = ""
	} else if err := rt.nodeProviderSecrets.SetValue(r.Context(), acct.CredentialRef, store.NodeProviderTokenEnvKey, value); err != nil {
		rt.internalError(w, "api: seal cloud account credential failed", err, slog.String("account", acct.ID))
		return
	}
	if err := st.CreateCloudAccount(r.Context(), acct); err != nil {
		rt.internalError(w, "api: create cloud account failed", err, slog.String("account", acct.ID))
		return
	}
	rt.nodeProviderCatalog.invalidate(acct.Provider)
	got, _ := st.GetCloudAccount(r.Context(), acct.ID)
	rt.auditCloud(r, "cloud.account.created", cloudAccountAuditDetail(acct))
	writeJSON(w, http.StatusCreated, rt.toCloudAccountWire(r.Context(), got))
}

type updateCloudAccountRequest struct {
	Label  *string           `json:"label,omitempty"`
	Status *string           `json:"status,omitempty"`
	Config map[string]string `json:"config,omitempty"`
	// ExternalID replaces the sealed value of a role_assumption account.
	ExternalID string `json:"external_id,omitempty"`
	// AllowedRegions replaces the region allowlist; an empty list clears it.
	AllowedRegions *[]string `json:"allowed_regions,omitempty"`
	// SessionToken and PrivateKeyPEM replace the stored browser session of an oci_session account.
	SessionToken  string `json:"session_token,omitempty"`
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`
}

func (rt *Router) handleUpdateCloudAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	var req updateCloudAccountRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label != nil {
		if len(*req.Label) > nodeProviderMaxMetaLen {
			writeError(w, http.StatusBadRequest, "label is limited to 500 characters")
			return
		}
		a.DisplayName = strings.TrimSpace(*req.Label)
	}
	if req.Status != nil {
		if *req.Status != store.CloudAccountStatusActive && *req.Status != store.CloudAccountStatusDisabled {
			writeError(w, http.StatusBadRequest, "status must be active or disabled")
			return
		}
		a.Status = *req.Status
		if a.Status == store.CloudAccountStatusDisabled {
			rt.revokeVaultAccount(r.Context(), a)
			rt.revokeDOOAuthAccount(r.Context(), a)
		}
	}
	if len(req.Config) > 0 || req.AllowedRegions != nil {
		cfg := map[string]any{}
		_ = json.Unmarshal(a.Config, &cfg)
		for k, v := range req.Config {
			if k == "allowed_regions" {
				continue
			}
			cfg[k] = v
		}
		if req.AllowedRegions != nil {
			list, status, msg := rt.validAllowedRegions(r.Context(), a, *req.AllowedRegions)
			if status != 0 {
				writeError(w, status, msg)
				return
			}
			if len(list) == 0 {
				delete(cfg, "allowed_regions")
			} else {
				cfg["allowed_regions"] = list
			}
		}
		a.Config, _ = json.Marshal(cfg)
	}
	if req.ExternalID != "" {
		if !isAWSRoleAccount(a) || rt.nodeProviderSecrets == nil {
			writeError(w, http.StatusBadRequest, "external_id applies only to aws role_assumption accounts")
			return
		}
		if a.CredentialRef == "" {
			a.CredentialRef = cloudAccountSecretPref + a.ID
		}
		if err := rt.nodeProviderSecrets.SetValue(r.Context(), a.CredentialRef, store.NodeProviderTokenEnvKey, req.ExternalID); err != nil {
			rt.internalError(w, "api: replace cloud account credential failed", err, slog.String("account", a.ID))
			return
		}
	}
	if req.SessionToken != "" || req.PrivateKeyPEM != "" {
		if !isOracleSessionAccount(a) || rt.nodeProviderSecrets == nil {
			writeError(w, http.StatusBadRequest, "session_token and private_key_pem apply only to oracle oci_session accounts")
			return
		}
		var msg string
		if a, msg = rt.replaceOracleSession(r.Context(), a, req.SessionToken, req.PrivateKeyPEM); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if err := st.UpdateCloudAccount(r.Context(), a); err != nil {
		rt.internalError(w, "api: update cloud account failed", err, slog.String("account", a.ID))
		return
	}
	rt.nodeProviderCatalog.invalidate(a.Provider)
	rt.providerCaps.invalidate(a.Provider)
	rt.auditCloud(r, "cloud.account.updated", cloudAccountAuditDetail(a))
	writeJSON(w, http.StatusOK, rt.toCloudAccountWire(r.Context(), a))
}

// validAllowedRegions dedupes the list and checks every id against what the provider lists.
func (rt *Router) validAllowedRegions(ctx context.Context, a store.CloudAccount, ids []string) ([]string, int, string) {
	if len(ids) == 0 {
		return nil, 0, ""
	}
	cctx, cancel := context.WithTimeout(ctx, nodeProviderTestTimeout)
	defer cancel()
	known, status, msg := rt.accountRegions(cctx, a)
	if status != 0 {
		return nil, status, msg
	}
	valid := map[string]bool{}
	for _, k := range known {
		valid[k.ID] = true
	}
	out := []string{}
	for _, id := range ids {
		if !valid[id] {
			return nil, http.StatusBadRequest, "allowed_regions has a region the provider does not list for this account"
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, 0, ""
}

func (rt *Router) handleDeleteCloudAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	st, _ := rt.cloudAcctStore()
	if r.URL.Query().Get("force") != "true" && rt.nodeProvisions != nil {
		provs, err := rt.nodeProvisions.ListNodeProvisions(r.Context())
		if err != nil {
			rt.internalError(w, "api: list provisions for account delete failed", err)
			return
		}
		for _, p := range provs {
			if p.AccountID == a.ID {
				writeError(w, http.StatusConflict, "provisions still reference this account, pass force=true to delete it anyway")
				return
			}
		}
	}
	rt.revokeVaultAccount(r.Context(), a)
	rt.revokeDOOAuthAccount(r.Context(), a)
	if err := st.DeleteCloudAccount(r.Context(), a.ID); err != nil {
		rt.internalError(w, "api: delete cloud account failed", err, slog.String("account", a.ID))
		return
	}
	if del, ok := rt.nodeProviderSecrets.(interface {
		DeleteAll(ctx context.Context, serviceName string) error
	}); ok && rt.nodeProviderSecrets != nil {
		if err := del.DeleteAll(r.Context(), cloudAccountSecretRef(a)); err != nil {
			rt.logger.Warn("api: delete cloud account credential failed", slog.String("account", a.ID))
		}
	}
	rt.nodeProviderCatalog.invalidate(a.Provider)
	rt.providerCaps.invalidate(a.Provider)
	rt.auditCloud(r, "cloud.account.deleted", cloudAccountAuditDetail(a))
	w.WriteHeader(http.StatusNoContent)
}

// cloudCheckResult is the verify outcome; texts are fixed, provider output never reaches the client.
type cloudCheckResult struct {
	OK       bool   `json:"ok"`
	Class    string `json:"class"`
	Message  string `json:"message"`
	Identity *struct {
		Account string `json:"account"`
		ARN     string `json:"arn"`
	} `json:"identity,omitempty"`
	Needs string `json:"needs,omitempty"`
}

var cloudCheckMessages = map[string]string{
	cloudconn.ClassOK:                   "The provider accepted the credential for a read-only call.",
	cloudconn.ClassAuthFailed:           "The provider rejected the credential.",
	cloudconn.ClassUnreachable:          "The provider endpoint could not be reached.",
	cloudconn.ClassConfigInvalid:        "The account settings do not validate.",
	cloudconn.ClassCredentialUnresolved: "The credential could not be read.",
	cloudconn.ClassUnknown:              "The check failed for an unclassified reason.",
	cloudconn.ClassNeedsRelogin:         "The browser session expired or is about to; log in again and replace the session.",
}

func (rt *Router) awsProvider() *cloudconn.AWSProvider {
	if p, ok := rt.cloudProviders[cloudconn.ProviderAWS].(*cloudconn.AWSProvider); ok {
		return p
	}
	return cloudconn.NewAWSProvider(cloudconn.AWSOptions{})
}

func awsTarget(a store.CloudAccount, credential string) cloudconn.Target {
	var cfg cloudconn.Config
	_ = json.Unmarshal(a.Config, &cfg)
	return cloudconn.Target{
		Connection: cloudconn.Connection{Name: a.ID, Provider: a.Provider, AuthMethod: a.AuthMethod, Config: cfg, CreatedBy: a.CreatedBy},
		Credential: secretsource.NewValue([]byte(credential), ""),
	}
}

// checkCloudAccount runs one read-only provider call with the candidate credential value.
func (rt *Router) checkCloudAccount(ctx context.Context, a store.CloudAccount, value string) cloudCheckResult {
	if isAWSBrokeredAccount(a) {
		t := awsTarget(a, value)
		p := rt.awsProvider()
		class, err := p.Check(ctx, t)
		if class == "" {
			class = cloudconn.ClassUnknown
		}
		res := cloudCheckResult{OK: err == nil && class == cloudconn.ClassOK, Class: class, Message: cloudCheckMessages[class]}
		if res.OK {
			if id, _, ierr := p.Identity(ctx, t); ierr == nil {
				res.Identity = &struct {
					Account string `json:"account"`
					ARN     string `json:"arn"`
				}{id.Account, id.ARN}
			}
		}
		if res.Message == "" {
			res.Message = cloudCheckMessages[cloudconn.ClassUnknown]
		}
		return res
	}
	if isOracleAccount(a) {
		return rt.oracleCheckResult(ctx, a, value)
	}
	ok, _, scope := rt.testNodeProviderScope(ctx, a.Provider, value)
	if ok {
		return cloudCheckResult{OK: true, Class: cloudconn.ClassOK, Message: cloudCheckMessages[cloudconn.ClassOK]}
	}
	res := cloudCheckResult{Class: cloudconn.ClassAuthFailed, Message: cloudCheckMessages[cloudconn.ClassAuthFailed]}
	if scope != nil {
		res.Needs = scope.Needs
	}
	return res
}

func (rt *Router) accountCredential(ctx context.Context, a store.CloudAccount) (string, error) {
	if isAWSVaultAccount(a) {
		return rt.vaultAccountToken(ctx, a)
	}
	if rt.nodeProviderSecrets == nil {
		return "", errors.New("no secrets backend")
	}
	if isDigitalOceanOAuthAccount(a) {
		return rt.doOAuthAccessToken(ctx, a)
	}
	return rt.nodeProviderSecrets.Resolve(ctx, cloudAccountSecretRef(a), store.NodeProviderTokenEnvKey)
}

func (rt *Router) handleVerifyCloudAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	value, err := rt.accountCredential(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no credential is stored for this account")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	res := rt.checkCloudAccount(ctx, a, value)
	st, _ := rt.cloudAcctStore()
	if res.OK {
		if err := st.MarkCloudAccountVerified(r.Context(), a.ID, time.Now().UTC()); err != nil {
			rt.logger.Warn("api: mark cloud account verified failed", slog.String("account", a.ID))
		}
	}
	writeJSON(w, http.StatusOK, res)
}

type cloudRegionWire struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (rt *Router) handleCloudAccountRegions(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	out, status, msg := rt.accountRegions(ctx, a)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// accountRegions lists the regions the credential can see; a non zero status is the error to send.
func (rt *Router) accountRegions(ctx context.Context, a store.CloudAccount) ([]cloudRegionWire, int, string) {
	out := []cloudRegionWire{}
	if isAWSBrokeredAccount(a) {
		value, err := rt.accountCredential(ctx, a)
		if err != nil {
			return nil, http.StatusBadRequest, "no credential is stored for this account"
		}
		regions, err := rt.awsProvider().ListRegions(ctx, awsTarget(a, value))
		if err != nil {
			return nil, http.StatusBadGateway, "could not list regions at the provider"
		}
		for _, id := range regions {
			out = append(out, cloudRegionWire{ID: id, Name: id})
		}
		return out, 0, ""
	}
	if isOracleAccount(a) {
		if ids, err := rt.oracleAccountRegions(ctx, a); err == nil {
			for _, id := range ids {
				out = append(out, cloudRegionWire{ID: id, Name: id})
			}
			return out, 0, ""
		}
	}
	p, ok, status, msg := rt.resolveNodeProvisionerForAccount(ctx, a.Provider, a.ID)
	if !ok {
		return nil, status, msg
	}
	regions, err := p.ListRegions(ctx)
	if err != nil {
		return nil, http.StatusBadGateway, "could not list regions at the provider"
	}
	for _, g := range regions {
		out = append(out, cloudRegionWire{ID: g.ID, Name: g.Name})
	}
	return out, 0, ""
}

type probeCloudAccountRequest struct {
	Capabilities []string `json:"capabilities,omitempty"`
}

func (rt *Router) handleProbeCloudAccount(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	if !isAWSBrokeredAccount(a) && !isOracleAccount(a) {
		writeError(w, http.StatusBadRequest, "capability probes are available for aws role_assumption, vault_aws and oracle accounts")
		return
	}
	var req probeCloudAccountRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if isOracleAccount(a) {
		rt.probeOracleAccount(w, r, a, req.Capabilities)
		return
	}
	if _, err := cloudconn.ResolveCapabilities(req.Capabilities); err != nil {
		writeError(w, http.StatusBadRequest, "unknown capability")
		return
	}
	value, err := rt.accountCredential(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no credential is stored for this account")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	res, err := rt.awsProvider().ProbeCapabilities(ctx, awsTarget(a, value), req.Capabilities...)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the capability probe could not run, verify the account first")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": res})
}

type awsLinkKitRequest struct {
	Capabilities        []string `json:"capabilities,omitempty"`
	Regions             []string `json:"regions,omitempty"`
	RoleName            string   `json:"role_name,omitempty"`
	TrustedPrincipalARN string   `json:"trusted_principal_arn"`
	// GuardPolicyARN is a managed deny policy (for example a budget guard) attached to the role.
	GuardPolicyARN string `json:"guard_policy_arn,omitempty"`
	// TemplateURL is an https URL where the caller hosts template_yaml; it enables quick_create_url.
	TemplateURL string `json:"template_url,omitempty"`
	StackName   string `json:"stack_name,omitempty"`
	Region      string `json:"region,omitempty"`
	// HostedZoneIDs scope the dns and destructive tiers to named Route 53 zones.
	HostedZoneIDs []string `json:"hosted_zone_ids,omitempty"`
}

type awsLinkKitResponse struct {
	ExternalID     string `json:"external_id"`
	RoleName       string `json:"role_name"`
	TemplateYAML   string `json:"template_yaml"`
	QuickCreateURL string `json:"quick_create_url,omitempty"`
	Terraform      string `json:"terraform"`
	PolicyJSON     string `json:"policy_json"`
	GuardPolicyARN string `json:"guard_policy_arn,omitempty"`
}

// requireZonesForZoneTiers refuses the dns and destructive tiers without named zones, so the role never spans every zone.
func requireZonesForZoneTiers(capabilities, zones []string) error {
	if len(zones) > 0 {
		return nil
	}
	if len(capabilities) == 0 {
		return errors.New("hosted_zone_ids is required because every tier includes dns")
	}
	for _, c := range capabilities {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case cloudconn.CapDNS, cloudconn.CapDestructive:
			return errors.New("hosted_zone_ids is required for the dns and destructive tiers")
		}
	}
	return nil
}

func (rt *Router) handleAWSLinkKit(w http.ResponseWriter, r *http.Request) {
	var req awsLinkKitRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.GuardPolicyARN == "" {
		req.GuardPolicyARN = rt.doctorGuardPolicyARN()
	}
	if req.GuardPolicyARN != "" && !awsGuardPolicyRe.MatchString(req.GuardPolicyARN) {
		writeError(w, http.StatusBadRequest, "guard_policy_arn must be an IAM managed policy ARN")
		return
	}
	if err := requireZonesForZoneTiers(req.Capabilities, req.HostedZoneIDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, err := newCloudID("lr-")
	if err != nil {
		rt.internalError(w, "api: mint external id failed", err)
		return
	}
	in := cloudconn.TemplateInput{RoleName: req.RoleName, TrustPrincipalARN: req.TrustedPrincipalARN, ExternalID: ext, Capabilities: req.Capabilities, Regions: req.Regions, HostedZoneIDs: req.HostedZoneIDs, NamePrefix: rt.awsNamePrefix()}
	opt := cloudconn.PolicyOptions{RoleName: req.RoleName, Regions: req.Regions, HostedZoneIDs: req.HostedZoneIDs, NamePrefix: in.NamePrefix}
	yaml, err := cloudconn.CloudFormationTemplate(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the link kit inputs are not valid")
		return
	}
	tf, err := cloudconn.TerraformSnippet(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the link kit inputs are not valid")
		return
	}
	policy, err := cloudconn.PolicyJSON(req.Capabilities, opt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the link kit inputs are not valid")
		return
	}
	roleName := req.RoleName
	if roleName == "" {
		roleName = cloudconn.DefaultPolicyOptions().RoleName
	}
	if g := req.GuardPolicyARN; g != "" {
		yaml = strings.Replace(yaml, "      PermissionsBoundary: !Ref BoundaryPolicy\n",
			"      PermissionsBoundary: !Ref BoundaryPolicy\n      ManagedPolicyArns:\n        - "+g+"\n", 1)
		tf += "\nresource \"aws_iam_role_policy_attachment\" \"levelrail_guard\" {\n  role       = aws_iam_role.levelrail.name\n  policy_arn = \"" + g + "\"\n}\n"
	}
	res := awsLinkKitResponse{ExternalID: ext, RoleName: roleName, TemplateYAML: yaml, Terraform: tf, PolicyJSON: policy, GuardPolicyARN: req.GuardPolicyARN}
	if req.TemplateURL != "" {
		stack, region := req.StackName, req.Region
		if stack == "" {
			stack = "levelrail-access"
		}
		if region == "" {
			region = "us-east-1"
		}
		if res.QuickCreateURL, err = cloudconn.QuickCreateURL(in, req.TemplateURL, stack, region); err != nil {
			writeError(w, http.StatusBadRequest, "template_url must be https and region an AWS region")
			return
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// resolveNodeProvisionerForAccount builds a provisioner from one account's credential. An empty
// accountID means the provider's default account, falling back to the legacy per provider secret.
func (rt *Router) resolveNodeProvisionerForAccount(ctx context.Context, providerName, accountID string) (p NodeProvisioner, ok bool, status int, msg string) {
	if !isKnownNodeProvider(providerName) {
		return nil, false, http.StatusNotFound, "unknown provider"
	}
	if rt.nodeProviderSecrets == nil {
		return nil, false, http.StatusNotImplemented, "node provisioning is not configured on this control plane (no master key set)"
	}
	acct := store.CloudAccount{Provider: providerName}
	if st, has := rt.cloudAcctStore(); has {
		id := accountID
		if id == "" {
			id = store.DefaultCloudAccountID(providerName)
		}
		got, err := st.GetCloudAccount(ctx, id)
		switch {
		case err == nil:
			if got.Provider != providerName {
				return nil, false, http.StatusBadRequest, "account belongs to a different provider"
			}
			if got.Status == store.CloudAccountStatusDisabled {
				return nil, false, http.StatusConflict, "cloud account is disabled"
			}
			acct = got
		case errors.Is(err, store.ErrCloudAccountNotFound) && accountID != "":
			return nil, false, http.StatusNotFound, "cloud account not found"
		case errors.Is(err, store.ErrCloudAccountNotFound):
		default:
			return nil, false, http.StatusInternalServerError, "internal error"
		}
	} else if accountID != "" {
		return nil, false, http.StatusNotImplemented, "cloud accounts are not available on this store"
	}
	var token string
	var err error
	if isDigitalOceanOAuthAccount(acct) {
		token, err = rt.doOAuthAccessToken(ctx, acct)
	} else {
		token, err = rt.nodeProviderSecrets.Resolve(ctx, cloudAccountSecretRef(acct), store.NodeProviderTokenEnvKey)
	}
	if err != nil {
		return nil, false, http.StatusBadRequest, "no credential is stored for this provider yet"
	}
	if isAWSRoleAccount(acct) {
		var cfg cloudconn.Config
		_ = json.Unmarshal(acct.Config, &cfg)
		raw, _ := json.Marshal(provision.AWSCredentials{ //nolint:gosec // sealed-at-rest shape handed to the factory, never logged
			Region: cfg.Region, RoleARN: cfg.RoleARN, ExternalID: token, UseAmbientCredentials: true})
		token = string(raw)
	}
	factory := rt.nodeProvisionerFactory
	if factory == nil {
		factory = rt.defaultNodeProvisionerFactory
	}
	provisioner, err := factory(providerName, token)
	if err != nil {
		return nil, false, http.StatusInternalServerError, "internal error"
	}
	return provisioner, true, 0, ""
}

// ensureDefaultCloudAccount keeps "<provider>-default" in step with the legacy node-providers routes.
func (rt *Router) ensureDefaultCloudAccount(ctx context.Context, provider, method, label, actor string) {
	st, ok := rt.cloudAcctStore()
	if !ok || provider == nodeProviderCustom {
		return
	}
	id := store.DefaultCloudAccountID(provider)
	if label == "" {
		label = provider
	}
	a, err := st.GetCloudAccount(ctx, id)
	if errors.Is(err, store.ErrCloudAccountNotFound) {
		err = st.CreateCloudAccount(ctx, store.CloudAccount{ID: id, Provider: provider, DisplayName: label, AuthMethod: method,
			CredentialRef: store.NodeProviderSecretsKey(provider), CreatedBy: actor})
	} else if err == nil {
		a.DisplayName, a.AuthMethod, a.CredentialRef, a.Status = label, method, store.NodeProviderSecretsKey(provider), store.CloudAccountStatusActive
		err = st.UpdateCloudAccount(ctx, a)
	}
	if err != nil {
		rt.logger.Warn("api: sync default cloud account failed", slog.String("account", id), slog.String("error", err.Error()))
	}
}

func (rt *Router) removeDefaultCloudAccount(ctx context.Context, provider string) {
	if st, ok := rt.cloudAcctStore(); ok {
		if err := st.DeleteCloudAccount(ctx, store.DefaultCloudAccountID(provider)); err != nil && !errors.Is(err, store.ErrCloudAccountNotFound) {
			rt.logger.Warn("api: remove default cloud account failed", slog.String("provider", provider))
		}
	}
}

// cloudAccountAuditDetail names the account only; config and credentials never enter the row.
func cloudAccountAuditDetail(a store.CloudAccount) map[string]any {
	return map[string]any{"account": a.ID, "provider": a.Provider, "team": a.TeamID, "status": a.Status}
}

// auditCloud writes one detail row beside the request row; details name scope and principals, never secrets.
func (rt *Router) auditCloud(r *http.Request, action string, detail map[string]any) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{ID: id, ActorType: auditActorSystem, Ability: AbilityWriteSensitive, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action, Detail: string(raw)}
	if pt, pid, _, perr := rt.callerPrincipal(r); perr == nil {
		entry.ActorType, entry.ActorID = "session", pid
		if pt == store.PrincipalTypeToken {
			entry.ActorType = "token"
		}
		entry.ActorName = rt.auditActorName(r.Context(), entry.ActorType, pid, pid)
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: save cloud audit entry failed", slog.String("action", action))
	}
}
