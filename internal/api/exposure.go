package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ExposureStore is the store surface the exposure handlers need.
type ExposureStore interface {
	SaveExposureRestriction(ctx context.Context, r store.ExposureRestriction) error
	ListExposureRestrictions(ctx context.Context) ([]store.ExposureRestriction, error)
	DeleteExposureRestriction(ctx context.Context, port int, protocol string) error
}

// WithExposure enables the exposure audit and its guided restrictions.
// Without it, the exposure routes return 501 and the doctor group is absent.
func WithExposure(m *exposure.Manager, s ExposureStore) Option {
	return func(rt *Router) {
		rt.exposure = m
		rt.exposureStore = s
	}
}

// HostFirewallSSHPorts exposes the SSH ports the firewall switch keeps open,
// so the exposure lockout guard protects the same set.
func HostFirewallSSHPorts() []int { return hostFirewallSSHPorts() }

// Outside-check outcomes. There is deliberately no "closed": a failed dial
// from this side proves nothing.
const (
	outsideNotRun   = "not_run"
	outsideAnswers  = "answers"
	outsideNoAnswer = "could_not_confirm"

	exposureNodeOK          = "ok"
	exposureNodeUnreachable = "unreachable"

	// localContainerSource is the Docker default address pool, the one range
	// "only local containers" can honestly stand for without reading networks.
	localContainerSource = "172.16.0.0/12"
	localContainersToken = "local-containers"
)

type exposureOutsideResource struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type exposureRestrictionResource struct {
	Allow     []string `json:"allow"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type exposureFindingResource struct {
	exposure.Finding
	Restriction *exposureRestrictionResource `json:"restriction,omitempty"`
	Outside     exposureOutsideResource      `json:"outside_check"`
}

type exposureNodeResource struct {
	NodeID        string                    `json:"node_id"`
	NodeName      string                    `json:"node_name"`
	Local         bool                      `json:"local"`
	Status        string                    `json:"status"`
	Error         string                    `json:"error,omitempty"`
	RulesReadable bool                      `json:"rules_readable"`
	RulesNote     string                    `json:"rules_note,omitempty"`
	PublicAddress string                    `json:"public_address,omitempty"`
	Findings      []exposureFindingResource `json:"findings"`
}

type exposureReportResource struct {
	GeneratedAt string                 `json:"generated_at"`
	Nodes       []exposureNodeResource `json:"nodes"`
	Exposed     int                    `json:"exposed"`
	High        int                    `json:"high"`
}

const localNodeLabel = "local"

// exposureOwners maps container names to the platform resource behind them.
func (rt *Router) exposureOwners(ctx context.Context) (map[string]exposure.Owner, error) {
	owners := map[string]exposure.Owner{}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	for _, svc := range services {
		pub := svc.BindAddress != "" && svc.BindAddress != bindaddr.Private
		for _, n := range orphans.DesiredServiceContainerNames(svc) {
			owners[n] = exposure.Owner{Kind: exposure.OwnerApp, Name: svc.Name, Intentional: pub}
		}
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	for _, db := range dbs {
		owners["db-"+db.Name] = exposure.Owner{Kind: exposure.OwnerDatabase, Name: db.Name, Intentional: db.PubliclyAccessible}
	}
	return owners, nil
}

func toExposureContainers(states []docker.ContainerState, owners map[string]exposure.Owner) []exposure.Container {
	out := make([]exposure.Container, 0, len(states))
	for _, s := range states {
		owner, ok := owners[s.Name]
		if !ok {
			owner = exposure.Owner{Kind: exposure.OwnerUnmanaged}
		}
		c := exposure.Container{ID: s.ID, Name: s.Name, Image: s.Image, Running: s.Running, Owner: owner}
		for _, p := range s.Ports {
			c.Ports = append(c.Ports, exposure.PortBinding{HostIP: p.HostIP, HostPort: p.HostPort, ContainerPort: p.ContainerPort, Protocol: p.Protocol})
		}
		out = append(out, c)
	}
	return out
}

func (rt *Router) exposureLister(nodeID string) (ContainerLister, error) {
	if rt.isLocalNode(nodeID) {
		if rt.containers != nil {
			return rt.containers, nil
		}
		if rt.execRuntime != nil {
			return rt.execRuntime("")
		}
		return nil, fmt.Errorf("container listing is not configured on this control plane")
	}
	if rt.execRuntime == nil {
		return nil, fmt.Errorf("node runtime routing is not configured")
	}
	return rt.execRuntime(nodeID)
}

type exposureNodeRef struct {
	id, name, address string
	local             bool
}

func (rt *Router) exposureNodeRefs(ctx context.Context, only string) []exposureNodeRef {
	refs := []exposureNodeRef{{id: "", name: localNodeLabel, local: true}}
	if rt.nodes == nil {
		return filterExposureNodes(refs, only)
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return filterExposureNodes(refs, only)
	}
	for _, n := range nodes {
		if rt.isLocalNode(n.ID) {
			refs[0].name = n.Name
			refs[0].id = n.ID
			continue
		}
		refs = append(refs, exposureNodeRef{id: n.ID, name: n.Name, address: n.Address})
	}
	return filterExposureNodes(refs, only)
}

func filterExposureNodes(refs []exposureNodeRef, only string) []exposureNodeRef {
	if only == "" {
		return refs
	}
	for _, r := range refs {
		if r.id == only || r.name == only || (only == localNodeLabel && r.local) {
			return []exposureNodeRef{r}
		}
	}
	return nil
}

// exposureAuditNode lists one node's published ports and classifies them.
// Rules are only readable for the control plane's own host.
func (rt *Router) exposureAuditNode(ctx context.Context, ref exposureNodeRef, owners map[string]exposure.Owner, chain exposure.Chain) exposureNodeResource {
	res := exposureNodeResource{NodeID: ref.id, NodeName: ref.name, Local: ref.local, Status: exposureNodeOK, Findings: []exposureFindingResource{}}
	lister, err := rt.exposureLister(ref.id)
	if err != nil {
		res.Status, res.Error = exposureNodeUnreachable, err.Error()
		return res
	}
	states, err := lister.ListByPrefix(ctx, "")
	if err != nil {
		res.Status, res.Error = exposureNodeUnreachable, err.Error()
		return res
	}
	nodeChain := exposure.Chain{Reason: "firewall rules on a remote node are not readable from the control plane yet, so a published port there cannot be proven restricted"}
	if ref.local {
		nodeChain = chain
	}
	res.RulesReadable, res.RulesNote = nodeChain.Readable, nodeChain.Reason
	restrictions := rt.exposureRestrictionMap(ctx)
	prefix := rt.exposure.Prefix()
	for _, f := range exposure.Audit(toExposureContainers(states, owners), nodeChain, prefix) {
		fr := exposureFindingResource{Finding: f, Outside: exposureOutsideResource{Status: outsideNotRun}}
		fr.CanRestrict, fr.CannotRestrictWhy = rt.exposureCanRestrict(f, ref, nodeChain)
		if r, ok := restrictions[restrictionKey(f.HostPort, f.Protocol)]; ok && ref.local {
			fr.Restriction = &exposureRestrictionResource{Allow: r.Allow, CreatedAt: r.CreatedAt}
		}
		res.Findings = append(res.Findings, fr)
	}
	return res
}

func restrictionKey(port int, proto string) string { return strconv.Itoa(port) + "/" + proto }

func (rt *Router) exposureRestrictionMap(ctx context.Context) map[string]store.ExposureRestriction {
	out := map[string]store.ExposureRestriction{}
	if rt.exposureStore == nil {
		return out
	}
	rows, err := rt.exposureStore.ListExposureRestrictions(ctx)
	if err != nil {
		rt.logger.Warn("api: exposure: list restrictions failed", "error", err.Error())
		return out
	}
	for _, r := range rows {
		out[restrictionKey(r.Port, r.Protocol)] = r
	}
	return out
}

func (rt *Router) exposureCanRestrict(f exposure.Finding, ref exposureNodeRef, chain exposure.Chain) (bool, string) {
	switch {
	case !ref.local:
		return false, "Rules can only be applied on the control plane's own host in this version. Run the dry-run commands on that node yourself."
	case f.Class != exposure.ClassExposed && f.Class != exposure.ClassUnknown:
		return false, ""
	case !chain.Readable:
		return false, "The DOCKER-USER chain cannot be read here, so a rule cannot be applied safely: " + chain.Reason
	}
	if _, err := rt.exposure.Plan(exposure.Restriction{Port: f.HostPort, Protocol: f.Protocol, Allow: []string{"192.0.2.1"}}); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// exposureReport builds the full audit. probe additionally dials each
// internet-facing port from this side.
func (rt *Router) exposureReport(ctx context.Context, only string, probe bool) (exposureReportResource, error) {
	owners, err := rt.exposureOwners(ctx)
	if err != nil {
		return exposureReportResource{}, fmt.Errorf("resolve owners: %w", err)
	}
	chain := rt.exposure.ReadChain(ctx)
	rep := exposureReportResource{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Nodes: []exposureNodeResource{}}
	for _, ref := range rt.exposureNodeRefs(ctx, only) {
		n := rt.exposureAuditNode(ctx, ref, owners, chain)
		if probe {
			rt.exposureProbeNode(ctx, ref, &n)
		}
		for _, f := range n.Findings {
			if f.Class == exposure.ClassExposed {
				rep.Exposed++
			}
			if f.NeedsAttention() && f.Severity == exposure.SeverityHigh {
				rep.High++
			}
		}
		rep.Nodes = append(rep.Nodes, n)
	}
	return rep, nil
}

// exposureProbeNode dials the node's public address on each internet-facing
// port. The local dial goes to this host's own public IP, so NAT hairpinning
// can fail it and a success does not prove the internet or a cloud firewall agrees.
func (rt *Router) exposureProbeNode(ctx context.Context, ref exposureNodeRef, n *exposureNodeResource) {
	host := ""
	if ref.local {
		pctx, cancel := context.WithTimeout(ctx, rt.doctorNetworkTimeoutOrDefault())
		defer cancel()
		endpoint := rt.doctorPublicIPEndpoint
		if endpoint == "" {
			endpoint = defaultDoctorPublicIPEndpoint
		}
		ip, err := doctorFetchPublicIP(pctx, rt.doctorHTTPClientOrDefault(), endpoint)
		if err == nil {
			host = ip
		}
	} else if h, _, err := net.SplitHostPort(ref.address); err == nil {
		host = h
	} else {
		host = ref.address
	}
	n.PublicAddress = host
	for i := range n.Findings {
		f := &n.Findings[i]
		if !f.NeedsAttention() && f.Class != exposure.ClassRestricted {
			continue
		}
		f.Outside = rt.exposureDial(ctx, host, f.HostPort, f.Protocol)
	}
}

func (rt *Router) exposureDial(ctx context.Context, host string, port int, proto string) exposureOutsideResource {
	if proto != "tcp" {
		return exposureOutsideResource{Status: outsideNoAnswer, Detail: "only TCP ports can be checked from here"}
	}
	if host == "" {
		return exposureOutsideResource{Status: outsideNoAnswer, Detail: "no public address is known for this node"}
	}
	dctx, cancel := context.WithTimeout(ctx, rt.doctorNetworkTimeoutOrDefault())
	defer cancel()
	conn, err := rt.doctorDialContextOrDefault()(dctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return exposureOutsideResource{Status: outsideNoAnswer, Detail: "could not confirm from this host; many routers block hairpin NAT even when a port is reachable, so this does not mean it is closed"}
	}
	_ = conn.Close()
	return exposureOutsideResource{Status: outsideAnswers, Detail: "the port answered on the public address from this side; a cloud firewall in front of the host may still block outside traffic"}
}

// handleExposureReport handles GET /api/v1/firewall/exposure.
func (rt *Router) handleExposureReport(w http.ResponseWriter, r *http.Request) {
	if rt.exposure == nil {
		writeError(w, http.StatusNotImplemented, "exposure audit is not configured on this control plane")
		return
	}
	q := r.URL.Query()
	rep, err := rt.exposureReport(r.Context(), strings.TrimSpace(q.Get("node")), q.Get("probe") == "true")
	if err != nil {
		rt.internalError(w, "api: exposure report failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
