package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// nodeProviderNames are the providers this control plane knows how to
// provision against. Adding a provider is a matter of an
// internal/provision.Provisioner implementation plus a new entry here.
var nodeProviderNames = []string{"hetzner", "digitalocean", "vultr", "aws", "azure", "gcp", "oracle"}

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

var controlPlaneAddrRe = regexp.MustCompile(`^[A-Za-z0-9.\-]+:[0-9]{1,5}$|^\[[0-9A-Fa-f:]+\]:[0-9]{1,5}$`)

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
	case "vultr":
		return provision.NewVultr(token), nil
	case "aws":
		return provision.NewAWSFromToken(token, rt.brand.ShortName)
	case "azure":
		return provision.NewAzure(token)
	case "gcp":
		return provision.NewGCP(token)
	case "oracle":
		return rt.newOracleProvisioner(token)
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
	if msg := rt.oracleDisabledMessage(r.Context(), provider); msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
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
		if rt.writeProviderScopeMissing(w, provider, "regions.read", err) {
			return
		}
		rt.logger.Error("api: list node provider regions failed", slog.String("provider", provider), slog.String("reason", providerErrorText(err)))
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
	if msg := rt.oracleDisabledMessage(r.Context(), provider); msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
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
		if rt.writeProviderScopeMissing(w, provider, "sizes.read", err) {
			return
		}
		rt.logger.Error("api: list node provider sizes failed", slog.String("provider", provider), slog.String("reason", providerErrorText(err)))
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
	return rt.resolveNodeProvisionerForAccount(ctx, providerName, "")
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
	// AccountID names the cloud account whose credential builds the VM; empty uses the provider default.
	AccountID string `json:"account_id,omitempty"`
	// BootstrapScriptID optionally names a saved bootstrap script run after the built-in bootstrap.
	BootstrapScriptID string `json:"bootstrap_script_id,omitempty"`
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
	if !controlPlaneAddrRe.MatchString(req.ControlPlaneAddr) {
		writeError(w, http.StatusBadRequest, "control_plane_addr must be host:port")
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
		rt.internalError(w, "api: node provision: check name collision failed", err)
		return
	} else if taken {
		writeError(w, http.StatusConflict, "a node or an in-progress provision already uses this name")
		return
	}

	provisioner, ok, status, msg := rt.resolveNodeProvisionerForAccount(r.Context(), req.Provider, req.AccountID)
	if !ok {
		writeError(w, status, msg)
		return
	}

	if sa, isSession := provisioner.(interface{ SessionAuth() bool }); isSession && sa.SessionAuth() {
		writeError(w, http.StatusBadRequest, "creating nodes needs an Oracle API key account; a browser session account can only read and manage instances")
		return
	}

	if reason := rt.checkProvisionSizeRegion(r.Context(), provisioner, req.Region, req.Size); reason != "" {
		writeError(w, http.StatusBadRequest, reason)
		return
	}

	if reason, err := rt.checkProvisionPriceCap(r.Context(), provisioner, req.Provider, req.Region, req.Size); err != nil {
		rt.internalError(w, "api: node provision: price check failed", err)
		return
	} else if reason != "" {
		writeError(w, http.StatusConflict, reason)
		return
	}

	if status, msg := preflightEC2Create(r.Context(), provisioner, provision.CreateOpts{Region: req.Region, Size: req.Size, Name: req.Name}); status != 0 {
		writeError(w, status, msg)
		return
	}

	var bootstrapBody string
	if req.BootstrapScriptID != "" {
		if rt.bootstrapScripts == nil {
			writeError(w, http.StatusNotImplemented, "bootstrap scripts are not configured on this control plane")
			return
		}
		bs, err := rt.bootstrapScripts.GetBootstrapScript(r.Context(), req.BootstrapScriptID)
		if errors.Is(err, store.ErrBootstrapScriptNotFound) {
			writeError(w, http.StatusBadRequest, "bootstrap_script_id does not match a saved bootstrap script")
			return
		}
		if err != nil {
			rt.internalError(w, "api: node provision: load bootstrap script failed", err)
			return
		}
		bootstrapBody = bs.Body
	}

	token, err := rt.mintNodeJoinToken(r.Context(), nodeProvisionJoinTokenTTL)
	if err != nil {
		rt.internalError(w, "api: node provision: mint join token failed", err)
		return
	}

	userData, err := provision.RenderCloudInit(provision.CloudInitParams{
		ControlPlaneAddr: req.ControlPlaneAddr,
		JoinToken:        token.plaintext,
		CAFingerprint:    rt.agentCAFingerprint,
		NodeName:         req.Name,
		AgentName:        rt.brand.AgentName(),
		DisplayName:      rt.brand.Name,
		AgentVersion:     version.Version,
		MeshEnabled:      rt.mesh != nil,

		RegistryMirrorPrefix: rt.registryMirrorPrefix(r.Context()),
	})
	if err != nil {
		rt.internalError(w, "api: node provision: render cloud-init failed", err)
		return
	}
	userData, err = provision.AppendBootstrapScript(userData, rt.brand.AgentName(), bootstrapBody)
	if err != nil {
		rt.internalError(w, "api: node provision: append bootstrap script failed", err)
		return
	}

	id, err := randomNodeProvisionID()
	if err != nil {
		rt.internalError(w, "api: node provision: generate id failed", err)
		return
	}
	// Live OCI counts are taken before the lock; only pending rows are rechecked under it.
	oracleUsage := rt.prefetchOracleUsage(r.Context(), provisioner, req.Provider, req.Region, req.Size)
	nodeProvisionGuardMu.Lock()
	reason, err := rt.checkOracleAllowanceFor(r.Context(), provisioner, req.Provider, req.AccountID, req.Region, req.Size, oracleUsage)
	if err == nil && reason == "" {
		reason, err = rt.checkProvisionServerCap(r.Context())
	}
	if err == nil && reason == "" {
		// Re-checked under the lock: the early check can race a concurrent create.
		var taken bool
		if taken, err = rt.nodeNameTaken(r.Context(), req.Name); err == nil && taken {
			reason = "a node or an in-progress provision already uses this name"
		}
	}
	if err != nil || reason != "" {
		nodeProvisionGuardMu.Unlock()
		if err != nil {
			rt.internalError(w, "api: node provision: server cap check failed", err)
			return
		}
		writeError(w, http.StatusConflict, reason)
		return
	}
	now := time.Now()
	rec := store.NodeProvision{
		ID: id, Provider: req.Provider, Region: req.Region, Size: req.Size, Name: req.Name, Role: role,
		Status: store.NodeProvisionStatusCreating, CreatedAt: now, UpdatedAt: now,
		AccountID: req.AccountID, CreatedBy: rt.nodeProviderActor(r),
	}
	saveErr := rt.nodeProvisions.SaveNodeProvision(r.Context(), rec)
	nodeProvisionGuardMu.Unlock()
	if err := saveErr; err != nil {
		rt.internalError(w, "api: node provision: save failed", err)
		return
	}

	// Detached from the caller: a dropped client must never leave a billed
	// server with no recorded id.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), envDurationOr(nodeProvisionCreateTimeoutEnv, defaultNodeProvisionCreateTimeout))
	defer cancel()
	fromCreating := func(p store.NodeProvision) bool { return p.Status == store.NodeProvisionStatusCreating }
	serverID, ipAddr, err := provisioner.CreateServer(cctx, provision.CreateOpts{
		Region: req.Region, Size: req.Size, Name: req.Name, UserData: userData, ProvisionID: id,
		AllowSSHInbound: req.AllowSSHInbound,
	})
	if err != nil {
		rt.logger.Error("api: node provision: create server failed", slog.String("provision_id", id), slog.Int("provider_status", providerErrorStatus(err)))
		failure := "could not create the server: " + providerErrorText(err)
		if serverID == "" {
			serverID, failure = rt.findTaggedServer(cctx, provisioner, req.Region, id, failure)
		}
		if serverID != "" {
			failure = rt.deleteOrphanedServer(cctx, provisioner, id, serverID, failure)
		}
		if _, uerr := rt.transitionNodeProvision(cctx, id, fromCreating, store.NodeProvisionStatusFailed, serverID, "", "", failure); uerr != nil {
			rt.logger.Error("api: node provision: record failure failed", slog.String("provision_id", id), slog.String("error", uerr.Error()))
		}
		if rt.writeProviderScopeMissing(w, req.Provider, "instances.create", err) {
			return
		}
		writeError(w, http.StatusBadGateway, "could not create the server at the provider")
		return
	}
	// A created server is the only proof a write scope exists.
	rt.recordProviderCapabilityLearned(req.Provider, "instances.create", nil)
	rec.ProviderServerID = serverID
	rec.IPAddress = ipAddr
	rec.Status = store.NodeProvisionStatusBooting
	if cur, err := rt.transitionNodeProvision(cctx, id, fromCreating, rec.Status, rec.ProviderServerID, rec.IPAddress, "", ""); err != nil {
		// The id may be missing from the row, so the server is deleted here
		// while it is still in memory; the id is recorded on the failed row either way.
		rt.logger.Error("api: node provision: record booting failed, deleting the orphaned server", slog.String("provision_id", id), slog.String("provider_server_id", serverID), slog.String("error", err.Error()))
		failure := rt.deleteOrphanedServer(cctx, provisioner, id, serverID, "created the server but could not record it")
		if uerr := rt.nodeProvisions.UpdateNodeProvisionStatus(cctx, id, store.NodeProvisionStatusFailed, serverID, "", "", failure, time.Now()); uerr != nil {
			rt.logger.Error("api: node provision: record failure failed", slog.String("provision_id", id), slog.String("error", uerr.Error()))
		}
		if errors.Is(err, errProvisionStatusChanged) && cur.Status == store.NodeProvisionStatusDestroyed {
			writeError(w, http.StatusConflict, "the provision was destroyed while it was being created")
			return
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
		if p.Name == name && p.Status != store.NodeProvisionStatusFailed && p.Status != store.NodeProvisionStatusDestroyed {
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
		rt.internalError(w, "api: list node provisions failed", err)
		return
	}
	rt.reconcileStaleNodeProvisions(r.Context(), provisions)
	if fresh, err := rt.nodeProvisions.ListNodeProvisions(r.Context()); err == nil {
		provisions = fresh
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
		rt.internalError(w, "api: get node provision failed", err, slog.String("provision_id", id))
		return
	}

	if nodeProvisionSettled(p.Status) {
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
		if n, ok := findProvisionedNode(nodes, p.Name, p.CreatedAt); ok {
			// Enrollment (internal/agent/server.go, frozen) always sets
			// AcceptsAppWorkloads true and leaves AcceptsBuildWorkloads
			// false, with no way to carry the operator's chosen Role
			// through the join-token exchange itself: apply it here,
			// the first point after enrollment this handler controls.
			acceptsApp, acceptsBuild := p.Role != "build", p.Role == "build"
			if werr := rt.nodes.UpdateNodeWorkloads(ctx, n.ID, acceptsApp, acceptsBuild); werr != nil {
				rt.logger.Error("api: node provision: apply role to node failed", slog.String("provision_id", p.ID), slog.String("node_id", n.ID), slog.String("error", werr.Error()))
				return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusEnrolling, p.IPAddress, "", "node enrolled but its role could not be applied yet, retrying: "+werr.Error())
			}
			rt.fillProvisionedDomain(ctx, p, n)
			return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusReady, p.IPAddress, n.ID, "")
		}
	} else {
		rt.logger.Warn("api: node provision: list nodes failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
	}

	if timeout := rt.nodeProvisionTimeout(ctx); time.Since(p.CreatedAt) > timeout {
		reason := fmt.Sprintf("timed out after %s waiting for the node to enroll", timeout)
		if p.ProviderServerID != "" {
			provisioner, ok, _, _ := rt.resolveNodeProvisioner(ctx, p.Provider)
			if !ok {
				return rt.saveNodeProvisionUpdate(ctx, p, p.Status, p.IPAddress, "", reason+"; the provider token is unavailable so the server was not deleted, delete server "+p.ProviderServerID+" in the provider console")
			}
			if derr := deleteRecordedServer(ctx, provisioner, p.ProviderServerID); derr != nil {
				rt.logger.Error("api: node provision: delete timed-out server failed", slog.String("provision_id", p.ID), slog.String("server_id", p.ProviderServerID), slog.Int("provider_status", providerErrorStatus(derr)))
				return rt.saveNodeProvisionUpdate(ctx, p, p.Status, p.IPAddress, "", reason+"; deleting the server failed, retrying: "+providerErrorText(derr))
			}
			reason += provisionServerDeletedNote
		}
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusFailed, p.IPAddress, "", reason)
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
		// Deleted like the timeout path: an errored server still bills.
		reason := "the provider reports this server errored"
		if derr := deleteRecordedServer(ctx, provisioner, p.ProviderServerID); derr != nil {
			rt.logger.Error("api: node provision: delete errored server failed", slog.String("provision_id", p.ID), slog.String("server_id", p.ProviderServerID), slog.Int("provider_status", providerErrorStatus(derr)))
			return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusFailed, ipAddr, "", reason+"; deleting the server failed, retrying: "+providerErrorText(derr))
		}
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusFailed, ipAddr, "", reason+provisionServerDeletedNote)
	default:
		return rt.saveNodeProvisionUpdate(ctx, p, store.NodeProvisionStatusBooting, ipAddr, "", "")
	}
}

// saveNodeProvisionUpdate persists a refresh result only while the row is
// still unsettled, so a stale poll can never overwrite a destroy or a result.
func (rt *Router) saveNodeProvisionUpdate(ctx context.Context, p store.NodeProvision, status, ipAddr, nodeID, failureReason string) store.NodeProvision {
	if status == p.Status && ipAddr == p.IPAddress && nodeID == p.NodeID && failureReason == p.FailureReason {
		return p
	}
	cur, err := rt.transitionNodeProvision(ctx, p.ID, provisionNotSettled, status, p.ProviderServerID, ipAddr, nodeID, failureReason)
	if errors.Is(err, errProvisionStatusChanged) {
		return cur
	}
	if err != nil {
		rt.logger.Error("api: node provision: update status failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
		return p
	}
	return cur
}

func (rt *Router) nodeProvisionTimeout(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return envDurationOr(nodeProvisionTimeoutEnv, defaultNodeProvisionTimeout)
	}
	return rt.tunableDuration(ctx, runtimesettings.KeyProvisionEnrollTimeout, nodeProvisionTimeoutEnv, defaultNodeProvisionTimeout)
}

// randomNodeProvisionID mirrors randomBackupTargetID/randomNodeJoinTokenID's
// exact shape: 9 random bytes, URL-safe base64, a short type prefix.
func randomNodeProvisionID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate node provision id: %w", err)
	}
	return "npv_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// fillProvisionedDomain stamps provider, region and zone on a freshly enrolled
// node, keeping any value the operator already set.
func (rt *Router) fillProvisionedDomain(ctx context.Context, p store.NodeProvision, n store.Node) {
	region, zone := provision.FailureDomain(p.Provider, p.Region)
	provider, region, zone := firstSet(n.Provider, p.Provider), firstSet(n.Region, region), firstSet(n.Zone, zone)
	if provider == n.Provider && region == n.Region && zone == n.Zone {
		return
	}
	if err := rt.nodes.UpdateNodeFailureDomain(ctx, n.ID, provider, region, zone); err != nil {
		rt.logger.Warn("api: node provision: fill failure domain failed", slog.String("provision_id", p.ID), slog.String("node_id", n.ID), slog.String("error", err.Error()))
	}
}

func firstSet(current, fallback string) string {
	if current != "" {
		return current
	}
	return fallback
}

// deleteOrphanedServer deletes a server whose row may not hold its id and
// returns the failure text, noting the delete outcome.
func (rt *Router) deleteOrphanedServer(ctx context.Context, p NodeProvisioner, provisionID, serverID, failure string) string {
	if derr := deleteRecordedServer(ctx, p, serverID); derr != nil {
		rt.logger.Error("api: node provision: delete orphaned server failed, it may still be running and billable", slog.String("provision_id", provisionID), slog.String("provider_server_id", serverID), slog.Int("provider_status", providerErrorStatus(derr)))
		return failure + "; deleting the server failed, delete server " + serverID + " in the provider console"
	}
	return failure + provisionServerDeletedNote
}

// reconcileBatch bounds how many stale rows one list call may repair.
const reconcileBatch = 5

// reconcileStaleNodeProvisions runs on list: it fails Creating rows with no
// recorded server past the enroll timeout, and retries the delete of any
// recorded server that a failed or timed-out row never lost track of.
func (rt *Router) reconcileStaleNodeProvisions(ctx context.Context, rows []store.NodeProvision) {
	timeout := rt.nodeProvisionTimeout(ctx)
	grace := envDurationOr(nodeProvisionCreateTimeoutEnv, defaultNodeProvisionCreateTimeout)
	done := 0
	for _, p := range rows {
		if done >= reconcileBatch {
			return
		}
		switch {
		case p.Status == store.NodeProvisionStatusCreating && p.ProviderServerID == "" && time.Since(p.CreatedAt) > timeout+grace:
			done++
			reason := "the control plane stopped before the server was recorded, check the provider console for a server named " + p.Name
			_, _ = rt.transitionNodeProvision(ctx, p.ID, func(c store.NodeProvision) bool {
				return c.Status == store.NodeProvisionStatusCreating && c.ProviderServerID == ""
			}, store.NodeProvisionStatusFailed, "", "", "", reason)
		case provisionServerLeaked(p):
			done++
			rt.retryLeakedDelete(ctx, p)
		case !nodeProvisionSettled(p.Status) && p.ProviderServerID != "" && time.Since(p.CreatedAt) > timeout:
			done++
			rt.refreshNodeProvision(ctx, p)
		}
	}
}

func (rt *Router) retryLeakedDelete(ctx context.Context, p store.NodeProvision) {
	provisioner, ok, _, _ := rt.resolveNodeProvisioner(ctx, p.Provider)
	if !ok {
		return
	}
	if err := deleteRecordedServer(ctx, provisioner, p.ProviderServerID); err != nil {
		rt.logger.Error("api: node provision: retry delete failed", slog.String("provision_id", p.ID), slog.Int("provider_status", providerErrorStatus(err)))
		return
	}
	_, _ = rt.transitionNodeProvision(ctx, p.ID, provisionServerLeaked, store.NodeProvisionStatusFailed, "", p.IPAddress, "", p.FailureReason+provisionServerDeletedNote)
}

// oracleTagLookup is implemented by the Oracle driver only.
type oracleTagLookup interface {
	FindServerByTag(ctx context.Context, region, tag string) (string, error)
}

// findTaggedServer looks for an instance a failed create left behind, found by
// the provision id tag, so it can be deleted instead of billing unseen.
func (rt *Router) findTaggedServer(ctx context.Context, p NodeProvisioner, region, provisionID, failure string) (string, string) {
	l, ok := p.(oracleTagLookup)
	if !ok {
		return "", failure
	}
	id, err := l.FindServerByTag(ctx, region, provisionID)
	if err != nil {
		rt.logger.Error("api: node provision: orphan lookup failed", slog.String("provision_id", provisionID), slog.Int("provider_status", providerErrorStatus(err)))
		return "", failure + "; could not confirm no server was left, check the provider console for a server tagged " + provisionID
	}
	return id, failure
}
