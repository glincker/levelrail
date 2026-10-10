package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Reachability verdict levels.
const (
	verdictStopped    = "stopped"
	verdictPrivate    = "private"
	verdictRestricted = "restricted"
	verdictExposed    = "exposed"
	verdictUnknown    = "unknown"

	defaultBridgeName = "bridge"
	networkKindBridge = "default-bridge"
	networkKindApp    = "app"
	networkKindOther  = "other"
	protoTCP          = "tcp"
)

type databaseInternalAddress struct {
	Host    string `json:"host"`
	Address string `json:"address,omitempty"`
	Port    int    `json:"port"`
}

type databaseNetworkAttachment struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	Kind    string `json:"kind"`
}

type databaseClientResource struct {
	App           string `json:"app"`
	Via           string `json:"via"`
	ProjectID     string `json:"project_id,omitempty"`
	ProjectName   string `json:"project_name,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
	InScope       bool   `json:"in_scope"`
	ScopeReason   string `json:"scope_reason,omitempty"`
}

type databasePublishedResource struct {
	HostPort      int      `json:"host_port"`
	ContainerPort int      `json:"container_port"`
	Bind          []string `json:"bind"`
	Class         string   `json:"class"`
	Severity      string   `json:"severity,omitempty"`
	Explanation   string   `json:"explanation,omitempty"`
	Managed       bool     `json:"managed"`
}

type databaseVerdictResource struct {
	Level   string `json:"level"`
	Text    string `json:"text"`
	Clients int    `json:"clients"`
	Port    int    `json:"port,omitempty"`
}

type databaseRuleResource struct {
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
}

type databaseRulesResource struct {
	Port         int                    `json:"port,omitempty"`
	Protocol     string                 `json:"protocol,omitempty"`
	Active       bool                   `json:"active"`
	Allow        []databaseRuleResource `json:"allow"`
	Missing      []string               `json:"missing,omitempty"`
	Extra        []string               `json:"extra,omitempty"`
	CanRestrict  bool                   `json:"can_restrict"`
	CannotReason string                 `json:"cannot_restrict_reason,omitempty"`
}

type databaseScopeResource struct {
	Current       string `json:"current"`
	ProjectID     string `json:"project_id,omitempty"`
	ProjectName   string `json:"project_name,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

type databaseTLSResource struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Required  bool   `json:"required"`
	State     string `json:"state,omitempty"`
	Drift     bool   `json:"drift"`
}

type databaseNetworkResponse struct {
	Database  string                      `json:"database"`
	Running   bool                        `json:"running"`
	Internal  databaseInternalAddress     `json:"internal"`
	Networks  []databaseNetworkAttachment `json:"networks"`
	Published *databasePublishedResource  `json:"published,omitempty"`
	Clients   []databaseClientResource    `json:"clients"`
	Verdict   databaseVerdictResource     `json:"verdict"`
	Rules     databaseRulesResource       `json:"rules"`
	Scope     databaseScopeResource       `json:"scope"`
	TLS       databaseTLSResource         `json:"tls"`
	Caveats   []string                    `json:"caveats"`
}

func networkKind(name string) string {
	switch {
	case name == defaultBridgeName:
		return networkKindBridge
	case strings.Contains(name, "-app-"):
		return networkKindApp
	default:
		return networkKindOther
	}
}

// databaseClients lists the apps that reference name, with their placement
// and whether the database's current scope admits them.
func (rt *Router) databaseClients(ctx context.Context, d *store.DesiredDatabase, scope dbaccess.Scope) ([]databaseClientResource, dbaccess.Placement, error) {
	projectNames := rt.projectNames(ctx)
	place := dbaccess.Placement{Name: d.Name, ProjectID: d.ProjectID, ProjectName: projectNames[d.ProjectID]}
	if ref, err := rt.dbAccess.EnvironmentOfDatabase(ctx, d.Name); err == nil && ref != nil {
		place.EnvironmentID, place.EnvironmentName = ref.ID, ref.Name
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, place, err
	}
	out := []databaseClientResource{}
	for _, svc := range services {
		via := ""
		for _, ref := range svc.DatabaseEnv {
			if ref.Database == d.Name {
				via = "env"
			}
		}
		if att := svc.DatabaseAttachment; att != nil && att.DatabaseName == d.Name {
			via = "attachment"
		}
		if via == "" {
			continue
		}
		c := databaseClientResource{App: svc.Name, Via: via, ProjectID: svc.ProjectID, ProjectName: projectNames[svc.ProjectID], EnvironmentID: svc.EnvironmentID}
		if ref, err := rt.dbAccess.EnvironmentOfApp(ctx, svc.Name); err == nil && ref != nil {
			c.EnvironmentID, c.Environment = ref.ID, ref.Name
		}
		c.InScope, c.ScopeReason = dbaccess.Allows(scope, place, dbaccess.Placement{
			Name: svc.Name, ProjectID: svc.ProjectID, ProjectName: c.ProjectName, EnvironmentID: c.EnvironmentID, EnvironmentName: c.Environment,
		})
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].App < out[j].App })
	return out, place, nil
}

func (rt *Router) projectNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	if rt.projects == nil {
		return out
	}
	ps, err := rt.projects.ListProjects(ctx)
	if err != nil {
		return out
	}
	for _, p := range ps {
		out[p.ID] = p.Name
	}
	return out
}

// databaseFinding returns the exposure audit finding for the database's
// published port on the local node, when the audit is configured.
func (rt *Router) databaseFinding(ctx context.Context, d *store.DesiredDatabase) (*exposureFindingResource, exposure.Chain) {
	if rt.exposure == nil {
		return nil, exposure.Chain{}
	}
	owners, err := rt.exposureOwners(ctx)
	if err != nil {
		return nil, exposure.Chain{}
	}
	chain := rt.exposure.ReadChain(ctx)
	if !rt.isLocalNode(d.NodeID) {
		return nil, chain
	}
	ref := exposureNodeRef{name: localNodeLabel, local: true}
	node := rt.exposureAuditNode(ctx, ref, owners, chain)
	for i := range node.Findings {
		if node.Findings[i].Container == database.ContainerName(d.Name) && node.Findings[i].HostPort == d.PublicPort {
			return &node.Findings[i], chain
		}
	}
	return nil, chain
}

func buildVerdict(running bool, published *databasePublishedResource, clients []databaseClientResource) databaseVerdictResource {
	inScope := 0
	for _, c := range clients {
		if c.InScope {
			inScope++
		}
	}
	v := databaseVerdictResource{Clients: inScope}
	switch {
	case !running:
		v.Level, v.Text = verdictStopped, "The database is not running, so nothing can reach it."
	case published == nil:
		v.Level = verdictPrivate
		v.Text = privateText(inScope)
	default:
		v.Port = published.HostPort
		switch published.Class {
		case string(exposure.ClassExposed):
			v.Level, v.Text = verdictExposed, "Exposed to the internet on port "+strconv.Itoa(published.HostPort)+"."
		case string(exposure.ClassRestricted):
			v.Level, v.Text = verdictRestricted, "Published on port "+strconv.Itoa(published.HostPort)+", restricted to the allowed sources."
		case string(exposure.ClassLoopback), string(exposure.ClassPrivate):
			v.Level = verdictPrivate
			v.Text = "Published on port " + strconv.Itoa(published.HostPort) + " but only reachable from the host itself."
		default:
			v.Level, v.Text = verdictUnknown, "Published on port "+strconv.Itoa(published.HostPort)+"; the firewall could not be read, so exposure is unknown."
		}
	}
	return v
}

func privateText(clients int) string {
	switch clients {
	case 0:
		return "Private: no apps reference it and no port is published."
	case 1:
		return "Private: only 1 app can reach it."
	default:
		return "Private: only these " + strconv.Itoa(clients) + " apps can reach it."
	}
}

// handleGetDatabaseNetwork handles GET /api/v1/databases/{name}/network.
func (rt *Router) handleGetDatabaseNetwork(w http.ResponseWriter, r *http.Request) {
	if rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	settings, err := rt.dbAccess.GetDatabaseAccessSettings(ctx, d.Name)
	if err != nil {
		rt.internalError(w, "api: database network: settings failed", err, slog.String("name", d.Name))
		return
	}
	scope := dbaccess.Scope(settings.Scope)
	clients, place, err := rt.databaseClients(ctx, d, scope)
	if err != nil {
		rt.internalError(w, "api: database network: clients failed", err, slog.String("name", d.Name))
		return
	}
	state := rt.databaseContainerState(ctx, d)
	port, _ := database.ContainerPort(d.Engine)
	resp := databaseNetworkResponse{
		Database: d.Name, Running: state != nil && state.Running,
		Internal: databaseInternalAddress{Host: database.ContainerName(d.Name), Port: port},
		Networks: []databaseNetworkAttachment{}, Clients: clients, Caveats: []string{},
	}
	resp.Scope = databaseScopeResource{Current: string(scope), ProjectID: place.ProjectID, ProjectName: rt.projectNames(ctx)[place.ProjectID], EnvironmentID: place.EnvironmentID}
	if ref, err := rt.dbAccess.EnvironmentOfDatabase(ctx, d.Name); err == nil && ref != nil {
		resp.Scope.Environment = ref.Name
	}
	rt.fillNetworks(&resp, state, d)
	finding, chain := rt.databaseFinding(ctx, d)
	rt.fillPublished(&resp, d, finding)
	resp.Rules = rt.databaseRules(ctx, r, d, finding, chain)
	resp.Verdict = buildVerdict(resp.Running, resp.Published, clients)
	if resp.Verdict.Level == verdictPrivate && resp.Published == nil {
		resp.Verdict.Text = privateText(resp.Verdict.Clients)
	}
	rt.fillTLS(ctx, &resp, d, settings)
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) databaseContainerState(ctx context.Context, d *store.DesiredDatabase) *docker.ContainerState {
	if rt.execRuntime == nil {
		return nil
	}
	nodeRuntime, err := rt.execRuntime(d.NodeID)
	if err != nil {
		return nil
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	state, err := nodeRuntime.InspectByName(inspectCtx, database.ContainerName(d.Name))
	if err != nil {
		return nil
	}
	return state
}

func (rt *Router) fillNetworks(resp *databaseNetworkResponse, state *docker.ContainerState, d *store.DesiredDatabase) {
	if state == nil {
		return
	}
	for _, n := range state.Networks {
		resp.Networks = append(resp.Networks, databaseNetworkAttachment{Name: n.Name, Address: n.IPAddress, Kind: networkKind(n.Name)})
		if resp.Internal.Address == "" && n.Name == defaultBridgeName {
			resp.Internal.Address = n.IPAddress
		}
	}
	if resp.Internal.Address == "" && len(state.Networks) > 0 {
		resp.Internal.Address = state.Networks[0].IPAddress
	}
	if slices.ContainsFunc(state.Networks, func(n docker.NetworkEndpoint) bool { return n.Name == defaultBridgeName }) && d.NodeID == "" {
		resp.Caveats = append(resp.Caveats, "bridge_reachable")
	}
}

func (rt *Router) fillPublished(resp *databaseNetworkResponse, d *store.DesiredDatabase, f *exposureFindingResource) {
	if !d.PubliclyAccessible || d.PublicPort == 0 {
		return
	}
	cport, _ := database.ContainerPort(d.Engine)
	p := &databasePublishedResource{HostPort: d.PublicPort, ContainerPort: cport, Bind: []string{bindLabelShort(d.PublicBindAddress)}, Class: string(exposure.ClassUnknown)}
	if f != nil {
		p.Bind, p.Class, p.Severity, p.Explanation, p.Managed = f.Binds, string(f.Class), string(f.Severity), f.Explanation, f.Managed
	}
	resp.Published = p
}

func bindLabelShort(bind string) string {
	if bind == "" {
		return "private"
	}
	return bind
}

func (rt *Router) fillTLS(ctx context.Context, resp *databaseNetworkResponse, d *store.DesiredDatabase, s store.DatabaseAccessSettings) {
	resp.TLS = databaseTLSResource{Supported: d.Engine == store.EnginePostgres, Enabled: rt.databaseTLSEnabled(ctx, *d), Required: s.RequireTLS}
	if d.Engine == store.EngineRedis || d.Engine == store.EngineKeyDB || d.Engine == store.EngineDragonfly {
		resp.TLS.Required = resp.TLS.Enabled
		resp.TLS.State = dbaccess.TLSRequired
		return
	}
	if !resp.TLS.Supported || !resp.Running || rt.execRuntime == nil || d.NodeID != "" && !rt.isLocalNode(d.NodeID) {
		return
	}
	nodeRuntime, err := rt.execRuntime(d.NodeID)
	if err != nil {
		return
	}
	state := rt.databaseContainerState(ctx, d)
	if state == nil {
		return
	}
	pg := dbaccess.Postgres{Exec: nodeRuntime, ContainerID: state.ID, Admin: d.Name, Database: d.Name}
	cur, err := pg.TLSState(ctx)
	if err != nil {
		rt.logger.Warn("api: database network: tls state unreadable", slog.String("name", d.Name), slog.String("error", err.Error()))
		return
	}
	resp.TLS.State = cur
	resp.TLS.Drift = (cur == dbaccess.TLSRequired) != s.RequireTLS
}
