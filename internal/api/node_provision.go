package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/idgen"
	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// nodeProviderNames are the providers this control plane knows how to
// provision against. Adding a provider is a matter of an
// internal/provision.Provisioner implementation plus a new entry here.
var nodeProviderNames = []string{"hetzner", "digitalocean", "aws", "azure", "gcp"}

// unknownNodeProviderMessage is the shared "which providers exist" text
// every unknown-provider validation error uses, derived from
// nodeProviderNames so it can't drift from the list it describes.
func unknownNodeProviderMessage() string {
	return "unknown provider, want one of " + strings.Join(nodeProviderNames, ", ")
}

// nodeProvisionNameRe mirrors gitSourceDatabaseNamePattern's own
// convention: lowercase alphanumeric and hyphens, since this name also
// becomes the provider's own server hostname and the enrolling agent's
// APP_NODE_NAME.
var nodeProvisionNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// nodeProvisionCatalogTTL bounds how long a provider's region/size list
// is cached before the next request re-fetches it live: a technical
// cache lifetime for a slow-changing third-party catalog, not an
// operator-tunable policy threshold, the same reasoning nodeJoinTokenTTL
// gives for staying a constant.
const nodeProvisionCatalogTTL = 5 * time.Minute

// nodeProvisionTimeout is how long a provision can sit short of "ready"
// before it's reported as failed: real operator-tunable policy (some
// images take longer to enroll than others), so unlike the TTL above
// this is an env var.
const nodeProvisionTimeoutEnv = "APP_NODE_PROVISION_TIMEOUT"

const defaultNodeProvisionTimeout = 20 * time.Minute

// nodeProvisionJoinTokenTTL is how long the join token minted for a
// cloud provision stays redeemable: comfortably longer than
// defaultNodeProvisionTimeout, so a slow-booting VM (provider queue,
// Docker install, agent image pull) never silently loses its one-shot
// token before cloud-init even gets to redeem it. nodeJoinTokenTTL
// (nodes.go) stays 15 minutes for the manual flow, where an operator is
// watching and can mint a fresh one immediately if it lapses; here
// nobody is watching until the provision either succeeds or times out.
const nodeProvisionJoinTokenTTL = 60 * time.Minute

// NodeProvisioner is the surface internal/provision.Provisioner
// implementations satisfy: a narrow, consumer-defined interface so this
// package's own tests inject a fake instead of hitting a real cloud API.
type NodeProvisioner interface {
	ListRegions(ctx context.Context) ([]provision.Region, error)
	ListSizes(ctx context.Context, region string) ([]provision.Size, error)
	CreateServer(ctx context.Context, opts provision.CreateOpts) (serverID, ipAddr string, err error)
	GetServer(ctx context.Context, id string) (status provision.ServerStatus, ipAddr string, err error)
	DeleteServer(ctx context.Context, id string) error
}

// NodeProvisionerFactory builds a NodeProvisioner for a known provider
// name given its resolved API token. Overridable in this package's own
// tests via WithNodeProvisionerFactory, the same "seam, not an
// interface" shape gitSourceFetch/fetch already use elsewhere in this
// package.
type NodeProvisionerFactory func(provider, token string) (NodeProvisioner, error)

// defaultNodeProvisionerFactory is a Router method rather than a
// standalone function so the aws case can pass brand.Brand.ShortName
// through: AWS namespaces the security group it creates with it (the
// product name never appears in source, so it's passed in, not looked
// up, the convention internal/network.WithShortName establishes).
func (rt *Router) defaultNodeProvisionerFactory(provider, token string) (NodeProvisioner, error) {
	switch provider {
	case "hetzner":
		return provision.NewHetzner(token), nil
	case "digitalocean":
		return provision.NewDigitalOcean(token), nil
	case "aws":
		return provision.NewAWSFromToken(token, rt.brand.ShortName)
	case "azure":
		return provision.NewAzure(token)
	case "gcp":
		return provision.NewGCP(token)
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
}

// NodeProvisionStore is the store surface the node provisioning handlers
// need.
type NodeProvisionStore interface {
	SaveNodeProvision(ctx context.Context, p store.NodeProvision) error
	GetNodeProvision(ctx context.Context, id string) (store.NodeProvision, error)
	ListNodeProvisions(ctx context.Context) ([]store.NodeProvision, error)
	UpdateNodeProvisionStatus(ctx context.Context, id, status, providerServerID, ipAddress, nodeID, failureReason string, updatedAt time.Time) error
}

// NodeProviderSecrets is the surface the node provider credential
// handlers need from internal/secrets.Manager: unlike CloudflareDNSSecrets
// (write-only from this package's own perspective), this package does
// call Resolve, to authenticate the live provider API calls
// handleCreateNodeProvision and the catalog handlers below make.
// *secrets.Manager satisfies this structurally.
type NodeProviderSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

type providerCatalogEntry struct {
	expires time.Time
	regions []provision.Region
	sizes   map[string][]provision.Size
}

// providerCatalogCache holds the brief, in-memory, per-provider region/
// size cache nodeProviderCatalogTTL bounds. Not persisted: losing it on
// restart just means the next request re-fetches live, the same cost as
// a cold cache entry.
type providerCatalogCache struct {
	mu      sync.Mutex
	entries map[string]*providerCatalogEntry
}

func newProviderCatalogCache() *providerCatalogCache {
	return &providerCatalogCache{entries: map[string]*providerCatalogEntry{}}
}

func (c *providerCatalogCache) invalidate(provider string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, provider)
}

// nodeProviderResource is the wire shape for GET /api/v1/node-providers.
type nodeProviderResource struct {
	Provider string `json:"provider"`
	HasToken bool   `json:"has_token"`
}

// handleListNodeProviders handles GET /api/v1/node-providers: every
// known provider and whether a credential is stored for it. Cheap, no
// live provider API calls (those are handleListNodeProviderRegions/
// handleListNodeProviderSizes's job).
func (rt *Router) handleListNodeProviders(w http.ResponseWriter, r *http.Request) {
	out := make([]nodeProviderResource, 0, len(nodeProviderNames))
	for _, p := range nodeProviderNames {
		res := nodeProviderResource{Provider: p}
		if rt.nodeProviderSecrets != nil {
			exists, err := rt.nodeProviderSecrets.Exists(r.Context(), store.NodeProviderSecretsKey(p), store.NodeProviderTokenEnvKey)
			if err != nil {
				rt.logger.Error("api: check node provider token failed", slog.String("provider", p), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			res.HasToken = exists
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// setNodeProviderCredentialRequest is POST /api/v1/node-providers' body.
// For hetzner/digitalocean, Token is that provider's single bearer API
// token and every field below it is unused. AWS has no single token, so
// Token there means AccessKeyID and the fields below fill out
// provision.AWSCredentials, which handleSetNodeProviderCredential
// JSON-encodes into the very same Token storage slot
// (store.NodeProviderTokenEnvKey): one credential path, not a second
// one, per provider.
type setNodeProviderCredentialRequest struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
	// The remaining fields are aws-only.
	SecretAccessKey       string `json:"secret_access_key,omitempty"`
	SessionToken          string `json:"session_token,omitempty"`
	Region                string `json:"region,omitempty"`
	RoleARN               string `json:"role_arn,omitempty"`
	UseAmbientCredentials bool   `json:"use_ambient_credentials,omitempty"`
}

// handleSetNodeProviderCredential handles POST /api/v1/node-providers:
// stores (or replaces) one provider's credential. Returns 501 without
// nodeProviderSecrets configured (no master key).
func (rt *Router) handleSetNodeProviderCredential(w http.ResponseWriter, r *http.Request) {
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "node provisioning is not configured on this control plane (no master key set)")
		return
	}
	var req setNodeProviderCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !isKnownNodeProvider(req.Provider) {
		writeError(w, http.StatusBadRequest, unknownNodeProviderMessage())
		return
	}
	value, msg := nodeProviderCredentialValue(req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := rt.nodeProviderSecrets.SetValue(r.Context(), store.NodeProviderSecretsKey(req.Provider), store.NodeProviderTokenEnvKey, value); err != nil {
		rt.logger.Error("api: save node provider token failed", slog.String("provider", req.Provider), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.nodeProviderCatalog.invalidate(req.Provider)
	writeJSON(w, http.StatusOK, nodeProviderResource{Provider: req.Provider, HasToken: true})
}

// nodeProviderCredentialValue validates req and returns the string
// stored under store.NodeProviderTokenEnvKey for req.Provider: the raw
// token for hetzner/digitalocean, or a JSON-encoded
// provision.AWSCredentials for aws. msg is non-empty (and value unused)
// when req fails validation.
func nodeProviderCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	if req.Provider != "aws" {
		if req.Token == "" {
			return "", "token is required"
		}
		return req.Token, ""
	}
	creds := provision.AWSCredentials{
		AccessKeyID: req.Token, SecretAccessKey: req.SecretAccessKey, SessionToken: req.SessionToken,
		Region: req.Region, RoleARN: req.RoleARN, UseAmbientCredentials: req.UseAmbientCredentials,
	}
	if !creds.UseAmbientCredentials && (creds.AccessKeyID == "" || creds.SecretAccessKey == "") {
		return "", "an access key id (token) and secret_access_key are required, or set use_ambient_credentials"
	}
	encoded, err := json.Marshal(creds) //nolint:gosec // the encoded json is the value passed to nodeProviderSecrets.SetValue below, which encrypts it at rest; it is never logged
	if err != nil {
		return "", "internal error"
	}
	return string(encoded), ""
}

func isKnownNodeProvider(p string) bool {
	for _, name := range nodeProviderNames {
		if p == name {
			return true
		}
	}
	return false
}

type nodeProviderRegionResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// handleListNodeProviderRegions handles GET
// /api/v1/node-providers/{provider}/regions: live regions from the
// provider's own API, cached for nodeProvisionCatalogTTL.
func (rt *Router) handleListNodeProviderRegions(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	p, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), provider)
	if !ok {
		writeError(w, status, msg)
		return
	}

	rt.nodeProviderCatalog.mu.Lock()
	entry, cached := rt.nodeProviderCatalog.entries[provider]
	if cached && time.Now().Before(entry.expires) && entry.regions != nil {
		regions := entry.regions
		rt.nodeProviderCatalog.mu.Unlock()
		writeJSON(w, http.StatusOK, toNodeProviderRegionResources(regions))
		return
	}
	rt.nodeProviderCatalog.mu.Unlock()

	regions, err := p.ListRegions(r.Context())
	if err != nil {
		rt.logger.Error("api: list node provider regions failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the provider's API")
		return
	}
	rt.nodeProviderCatalog.mu.Lock()
	if entry == nil {
		entry = &providerCatalogEntry{sizes: map[string][]provision.Size{}}
		rt.nodeProviderCatalog.entries[provider] = entry
	}
	entry.regions = regions
	entry.expires = time.Now().Add(nodeProvisionCatalogTTL)
	rt.nodeProviderCatalog.mu.Unlock()

	writeJSON(w, http.StatusOK, toNodeProviderRegionResources(regions))
}

func toNodeProviderRegionResources(regions []provision.Region) []nodeProviderRegionResource {
	out := make([]nodeProviderRegionResource, 0, len(regions))
	for _, r := range regions {
		out = append(out, nodeProviderRegionResource{ID: r.ID, Name: r.Name})
	}
	return out
}

type nodeProviderSizeResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	VCPUs        int    `json:"vcpus"`
	MemoryMB     int    `json:"memory_mb"`
	DiskGB       int    `json:"disk_gb"`
	PriceMonthly string `json:"price_monthly,omitempty"`
	Currency     string `json:"currency,omitempty"`
}

// handleListNodeProviderSizes handles GET
// /api/v1/node-providers/{provider}/sizes?region=X: live sizes (with
// that region's own price when the provider returns one), cached for
// nodeProvisionCatalogTTL per (provider, region).
func (rt *Router) handleListNodeProviderSizes(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	region := r.URL.Query().Get("region")
	p, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), provider)
	if !ok {
		writeError(w, status, msg)
		return
	}

	rt.nodeProviderCatalog.mu.Lock()
	entry, cached := rt.nodeProviderCatalog.entries[provider]
	if cached && time.Now().Before(entry.expires) {
		if sizes, ok := entry.sizes[region]; ok {
			rt.nodeProviderCatalog.mu.Unlock()
			writeJSON(w, http.StatusOK, toNodeProviderSizeResources(sizes))
			return
		}
	}
	rt.nodeProviderCatalog.mu.Unlock()

	sizes, err := p.ListSizes(r.Context(), region)
	if err != nil {
		rt.logger.Error("api: list node provider sizes failed", slog.String("provider", provider), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the provider's API")
		return
	}
	rt.nodeProviderCatalog.mu.Lock()
	if entry == nil {
		entry = &providerCatalogEntry{sizes: map[string][]provision.Size{}}
		rt.nodeProviderCatalog.entries[provider] = entry
	}
	if entry.sizes == nil {
		entry.sizes = map[string][]provision.Size{}
	}
	entry.sizes[region] = sizes
	entry.expires = time.Now().Add(nodeProvisionCatalogTTL)
	rt.nodeProviderCatalog.mu.Unlock()

	writeJSON(w, http.StatusOK, toNodeProviderSizeResources(sizes))
}

func toNodeProviderSizeResources(sizes []provision.Size) []nodeProviderSizeResource {
	out := make([]nodeProviderSizeResource, 0, len(sizes))
	for _, s := range sizes {
		out = append(out, nodeProviderSizeResource{
			ID: s.ID, Name: s.Name, VCPUs: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk,
			PriceMonthly: s.PriceMonthly, Currency: s.Currency,
		})
	}
	return out
}

// resolveNodeProvisioner resolves provider's stored token and builds a
// NodeProvisioner from it, or reports the HTTP status/message a caller
// should return.
func (rt *Router) resolveNodeProvisioner(ctx context.Context, providerName string) (p NodeProvisioner, ok bool, status int, msg string) {
	if !isKnownNodeProvider(providerName) {
		return nil, false, http.StatusNotFound, "unknown provider"
	}
	if rt.nodeProviderSecrets == nil {
		return nil, false, http.StatusNotImplemented, "node provisioning is not configured on this control plane (no master key set)"
	}
	token, err := rt.nodeProviderSecrets.Resolve(ctx, store.NodeProviderSecretsKey(providerName), store.NodeProviderTokenEnvKey)
	if err != nil {
		return nil, false, http.StatusBadRequest, "no credential is stored for this provider yet"
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

// nodeProvisionResource is the wire shape for a node provision.
type nodeProvisionResource struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	Region        string    `json:"region"`
	Size          string    `json:"size"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	IPAddress     string    `json:"ip_address,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toNodeProvisionResource(p store.NodeProvision) nodeProvisionResource {
	return nodeProvisionResource{
		ID: p.ID, Provider: p.Provider, Region: p.Region, Size: p.Size, Name: p.Name, Role: p.Role,
		Status: p.Status, IPAddress: p.IPAddress, NodeID: p.NodeID, FailureReason: p.FailureReason,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type createNodeProvisionRequest struct {
	Provider string `json:"provider"`
	Region   string `json:"region"`
	Size     string `json:"size"`
	Name     string `json:"name"`
	// Role is general or build. Empty defaults to general.
	Role string `json:"role"`
	// ControlPlaneAddr is the host:port the new server dials to reach
	// this control plane's agent gRPC listener. Required: this process
	// cannot reliably learn its own externally reachable address (NAT, a
	// separate public hostname), the same reason AddNodeDialog.tsx's own
	// manual join-token flow asks the operator for it rather than
	// guessing server-side.
	ControlPlaneAddr string `json:"control_plane_addr"`
	// AllowSSHInbound is aws-only (provision.CreateOpts.AllowSSHInbound):
	// other providers ignore it. Off by default.
	AllowSSHInbound bool `json:"allow_ssh_inbound,omitempty"`
}

// handleCreateNodeProvision handles POST /api/v1/nodes/provision: mints
// a join token through the same path handleCreateNodeJoinToken uses,
// creates a server at the provider with a cloud-init script that
// enrolls it, and records a node_provisions row a caller polls via
// handleGetNodeProvision.
func (rt *Router) handleCreateNodeProvision(w http.ResponseWriter, r *http.Request) {
	var req createNodeProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !isKnownNodeProvider(req.Provider) {
		writeError(w, http.StatusBadRequest, unknownNodeProviderMessage())
		return
	}
	if req.Region == "" || req.Size == "" {
		writeError(w, http.StatusBadRequest, "region and size are required")
		return
	}
	if req.ControlPlaneAddr == "" {
		writeError(w, http.StatusBadRequest, "control_plane_addr is required")
		return
	}
	if !nodeProvisionNameRe.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must start with a lowercase letter and contain only lowercase letters, digits and hyphens")
		return
	}
	role, msg := normalizeNodeRole(req.Role)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if taken, err := rt.nodeNameTaken(r.Context(), req.Name); err != nil {
		rt.logger.Error("api: node provision: check name collision failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if taken {
		writeError(w, http.StatusConflict, "a node or an in-progress provision already uses this name")
		return
	}

	provisioner, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), req.Provider)
	if !ok {
		writeError(w, status, msg)
		return
	}

	token, err := rt.mintNodeJoinToken(r.Context(), nodeProvisionJoinTokenTTL)
	if err != nil {
		rt.logger.Error("api: node provision: mint join token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userData, err := provision.RenderCloudInit(provision.CloudInitParams{
		ControlPlaneAddr: req.ControlPlaneAddr,
		JoinToken:        token.plaintext,
		CAFingerprint:    rt.agentCAFingerprint,
		NodeName:         req.Name,
		AgentVersion:     version.Version,
	})
	if err != nil {
		rt.logger.Error("api: node provision: render cloud-init failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomNodeProvisionID()
	if err != nil {
		rt.logger.Error("api: node provision: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now()
	rec := store.NodeProvision{
		ID: id, Provider: req.Provider, Region: req.Region, Size: req.Size, Name: req.Name, Role: role,
		Status: store.NodeProvisionStatusCreating, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.nodeProvisions.SaveNodeProvision(r.Context(), rec); err != nil {
		rt.logger.Error("api: node provision: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	serverID, ipAddr, err := provisioner.CreateServer(r.Context(), provision.CreateOpts{
		Region: req.Region, Size: req.Size, Name: req.Name, UserData: userData,
		AllowSSHInbound: req.AllowSSHInbound,
	})
	if err != nil {
		rt.logger.Error("api: node provision: create server failed", slog.String("provision_id", id), slog.String("error", err.Error()))
		if uerr := rt.nodeProvisions.UpdateNodeProvisionStatus(r.Context(), id, store.NodeProvisionStatusFailed, "", "", "", "could not create the server: "+err.Error(), time.Now()); uerr != nil {
			rt.logger.Error("api: node provision: record failure failed", slog.String("provision_id", id), slog.String("error", uerr.Error()))
		}
		writeError(w, http.StatusBadGateway, "could not create the server at the provider")
		return
	}
	rec.ProviderServerID = serverID
	rec.IPAddress = ipAddr
	rec.Status = store.NodeProvisionStatusBooting
	rec.UpdatedAt = time.Now()
	if err := rt.nodeProvisions.UpdateNodeProvisionStatus(r.Context(), id, rec.Status, rec.ProviderServerID, rec.IPAddress, "", "", rec.UpdatedAt); err != nil {
		// The server exists at the provider but its ID never made it into
		// this row: without it, no later read can query or delete that
		// server again, an untracked, still-billable VM despite a
		// successful CreateServer call. Best-effort delete it now, while
		// this handler still has the ID in memory, rather than leave that
		// behind a 201 that claims otherwise.
		rt.logger.Error("api: node provision: record booting failed, deleting the orphaned server", slog.String("provision_id", id), slog.String("provider_server_id", serverID), slog.String("error", err.Error()))
		if derr := provisioner.DeleteServer(r.Context(), serverID); derr != nil {
			rt.logger.Error("api: node provision: delete orphaned server failed, it may still be running and billable", slog.String("provision_id", id), slog.String("provider_server_id", serverID), slog.String("error", derr.Error()))
		}
		if uerr := rt.nodeProvisions.UpdateNodeProvisionStatus(r.Context(), id, store.NodeProvisionStatusFailed, "", "", "", "created the server but could not record it, so it was deleted", time.Now()); uerr != nil {
			rt.logger.Error("api: node provision: record failure failed", slog.String("provision_id", id), slog.String("error", uerr.Error()))
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.logger.Info("api: node provision created", slog.String("provision_id", id), slog.String("provider", req.Provider), slog.String("name", req.Name))
	writeJSON(w, http.StatusCreated, toNodeProvisionResource(rec))
}

// normalizeNodeRole validates and normalizes a node provision's role
// field ("" defaults to "general"), shared by handleCreateNodeProvision
// and handleCreateSSHNodeProvision (node_ssh_provision.go) so the same
// general/build vocabulary and error wording can't drift between the
// cloud and SSH provisioning paths. msg is non-empty (and role unused)
// when role fails validation.
func normalizeNodeRole(role string) (normalized, msg string) {
	if role == "" {
		role = "general"
	}
	if role != "general" && role != "build" {
		return "", "role must be general or build"
	}
	return role, ""
}

// nodeNameTaken reports whether name is already used by an enrolled
// node, or by another cloud or SSH provision that has not yet failed:
// refreshNodeProvision/refreshSSHNodeProvision (node_ssh_provision.go)
// each match an enrolling provision to a real node purely by Name, so a
// second provision (or a pre-existing node) reusing that name would let
// it report false readiness against an unrelated machine.
func (rt *Router) nodeNameTaken(ctx context.Context, name string) (bool, error) {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return false, fmt.Errorf("list nodes: %w", err)
	}
	if _, ok := findNodeByName(nodes, name); ok {
		return true, nil
	}
	provisions, err := rt.nodeProvisions.ListNodeProvisions(ctx)
	if err != nil {
		return false, fmt.Errorf("list node provisions: %w", err)
	}
	for _, p := range provisions {
		if p.Name == name && p.Status != store.NodeProvisionStatusFailed {
			return true, nil
		}
	}
	sshProvisions, err := rt.sshProvisions.ListSSHNodeProvisions(ctx)
	if err != nil {
		return false, fmt.Errorf("list ssh node provisions: %w", err)
	}
	for _, p := range sshProvisions {
		if p.Name == name && p.Status != store.SSHNodeProvisionStatusFailed {
			return true, nil
		}
	}
	return false, nil
}

// handleListNodeProvisions handles GET /api/v1/node-provisions: every
// provision, last known status. Deliberately not live-refreshed here
// (that's handleGetNodeProvision's job): refreshing every row on every
// list load would mean one provider API call per row per poll, the same
// N+1 concern nodeResource's own AlertStatus doc comment gives for why
// it's absent from the node list response too.
func (rt *Router) handleListNodeProvisions(w http.ResponseWriter, r *http.Request) {
	provisions, err := rt.nodeProvisions.ListNodeProvisions(r.Context())
	if err != nil {
		rt.logger.Error("api: list node provisions failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]nodeProvisionResource, 0, len(provisions))
	for _, p := range provisions {
		out = append(out, toNodeProvisionResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetNodeProvision handles GET /api/v1/node-provisions/{id}:
// recomputes status live against the provider API and the real nodes
// table (by Name) before returning, the same "pull, not push" shape
// nodeCertAndAgent's own live re-evaluation uses.
func (rt *Router) handleGetNodeProvision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := rt.nodeProvisions.GetNodeProvision(r.Context(), id)
	if errors.Is(err, store.ErrNodeProvisionNotFound) {
		writeError(w, http.StatusNotFound, "node provision not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get node provision failed", slog.String("provision_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if p.Status == store.NodeProvisionStatusReady || p.Status == store.NodeProvisionStatusFailed {
		writeJSON(w, http.StatusOK, toNodeProvisionResource(p))
		return
	}

	updated := rt.refreshNodeProvision(r.Context(), p)
	writeJSON(w, http.StatusOK, toNodeProvisionResource(updated))
}

// refreshNodeProvision re-evaluates one non-terminal provision against
// the provider API and the node registry, persisting any change. Errors
// talking to the provider are logged and otherwise swallowed: a
// transient network blip must not flip a provision to failed, it should
// just be retried on the next poll.
func (rt *Router) refreshNodeProvision(ctx context.Context, p store.NodeProvision) store.NodeProvision {
	if nodes, err := rt.nodes.ListNodes(ctx); err == nil {
		if n, ok := findNodeByName(nodes, p.Name); ok {
			// Enrollment (internal/agent/server.go, frozen) always sets
			// AcceptsAppWorkloads true and leaves AcceptsBuildWorkloads
			// false, with no way to carry the operator's chosen Role
			// through the join-token exchange itself: apply it here,
			// the first point after enrollment this handler controls.
			acceptsApp, acceptsBuild := p.Role != "build", p.Role == "build"
			if werr := rt.nodes.UpdateNodeWorkloads(ctx, n.ID, acceptsApp, acceptsBuild); werr != nil {
				rt.logger.Error("api: node provision: apply role to node failed", slog.String("provision_id", p.ID), slog.String("node_id", n.ID), slog.String("error", werr.Error()))
			}
			return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusReady, p.IPAddress, n.ID, "")
		}
	} else {
		rt.logger.Warn("api: node provision: list nodes failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
	}

	if timeout := nodeProvisionTimeout(); time.Since(p.CreatedAt) > timeout {
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusFailed, p.IPAddress, "", fmt.Sprintf("timed out after %s waiting for the node to enroll", timeout))
	}

	provisioner, ok, _, _ := rt.resolveNodeProvisioner(ctx, p.Provider)
	if !ok || p.ProviderServerID == "" {
		return p
	}
	serverStatus, ipAddr, err := provisioner.GetServer(ctx, p.ProviderServerID)
	if err != nil {
		rt.logger.Warn("api: node provision: get server failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
		return p
	}
	if ipAddr == "" {
		ipAddr = p.IPAddress
	}

	switch serverStatus {
	case provision.ServerStatusRunning:
		// The server is up but the agent hasn't reported in yet: no
		// signal from inside the VM distinguishes "still running
		// cloud-init" from "waiting to dial the control plane", so
		// this control plane only ever reports "enrolling" here, never
		// the finer-grained "installing" store.NodeProvisionStatus also
		// defines for a future agent-reported signal to use.
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusEnrolling, ipAddr, "", "")
	case provision.ServerStatusError:
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusFailed, ipAddr, "", "the provider reports this server errored")
	default:
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusBooting, ipAddr, "", "")
	}
}

func (rt *Router) saveNodeProvisionUpdate(ctx context.Context, p store.NodeProvision, status, ipAddr, nodeID, failureReason string) store.NodeProvision {
	if status == p.Status && ipAddr == p.IPAddress && nodeID == p.NodeID && failureReason == p.FailureReason {
		return p
	}
	now := time.Now()
	if err := rt.nodeProvisions.UpdateNodeProvisionStatus(ctx, p.ID, status, p.ProviderServerID, ipAddr, nodeID, failureReason, now); err != nil {
		rt.logger.Error("api: node provision: update status failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
		return p
	}
	p.Status, p.IPAddress, p.NodeID, p.FailureReason, p.UpdatedAt = status, ipAddr, nodeID, failureReason, now
	return p
}

func nodeProvisionTimeout() time.Duration {
	return envDurationOr(nodeProvisionTimeoutEnv, defaultNodeProvisionTimeout)
}

// randomNodeProvisionID mints a random "npv_" ID via idgen.
func randomNodeProvisionID() (string, error) {
	id, err := idgen.New("npv_")
	if err != nil {
		return "", fmt.Errorf("api: generate node provision id: %w", err)
	}
	return id, nil
}
