package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

// errNoGPUTarget marks a GPU workload that no candidate node can host.
type errNoGPUTarget struct{ reason string }

func (e *errNoGPUTarget) Error() string { return e.reason }

// gpuPlanner answers "does this GPU claim fit on that node" from a
// snapshot of node GPU reports and reservations, tracking claims added
// during one bulk operation such as a drain.
type gpuPlanner struct {
	localID string
	nodes   map[string]models.GPUNode
	claims  map[string][]gpu.Claim
}

func (rt *Router) newGPUPlanner(ctx context.Context) (*gpuPlanner, error) {
	if rt.models == nil {
		return nil, nil
	}
	nodes, err := rt.models.GPUNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: list gpu nodes: %w", err)
	}
	p := &gpuPlanner{localID: rt.localNodeID, nodes: make(map[string]models.GPUNode, len(nodes)), claims: make(map[string][]gpu.Claim, len(nodes))}
	for _, n := range nodes {
		p.nodes[n.PlacementID] = n
		p.claims[n.PlacementID] = append([]gpu.Claim(nil), n.Claims...)
	}
	return p, nil
}

func (p *gpuPlanner) key(nodeID string) string {
	if nodeID == p.localID {
		return ""
	}
	return nodeID
}

func (p *gpuPlanner) fit(nodeID string, c gpu.Claim) error {
	k := p.key(nodeID)
	n, ok := p.nodes[k]
	if !ok {
		return gpu.ErrNoGPU
	}
	claims := make([]gpu.Claim, 0, len(p.claims[k]))
	for _, existing := range p.claims[k] {
		if existing.Name != c.Name {
			claims = append(claims, existing)
		}
	}
	return gpu.NewLedger(n.Info, claims).Fit(c)
}

func (p *gpuPlanner) reserve(nodeID string, c gpu.Claim) {
	k := p.key(nodeID)
	p.claims[k] = append(p.claims[k], c)
}

// pick returns the least loaded candidate the claim fits, or a
// *errNoGPUTarget listing why each one was rejected. candidates must
// already be filtered for schedulable and online.
func (p *gpuPlanner) pick(candidates []nodePlacementLoad, c gpu.Claim) (string, error) {
	fitting := make([]nodePlacementLoad, 0, len(candidates))
	var why []string
	for _, cand := range candidates {
		if err := p.fit(cand.NodeID, c); err != nil {
			why = append(why, fmt.Sprintf("%s: %s", p.label(cand.NodeID), err))
			continue
		}
		fitting = append(fitting, cand)
	}
	if len(fitting) == 0 {
		reason := "no GPU node available"
		if len(why) > 0 {
			reason += " (" + strings.Join(why, "; ") + ")"
		} else {
			reason += " (no other schedulable online node)"
		}
		return "", &errNoGPUTarget{reason: reason}
	}
	return selectLeastLoadedNode(fitting), nil
}

func (p *gpuPlanner) label(nodeID string) string {
	if n, ok := p.nodes[p.key(nodeID)]; ok {
		return n.Name
	}
	if nodeID == "" {
		return models.LocalNodeName
	}
	return nodeID
}

// spread picks a node for a GPU claim: the least loaded registered node
// it fits, else the local host (unless exclude is the local host).
func (p *gpuPlanner) spread(nodes []store.Node, counts map[string]int, exclude string, c gpu.Claim) (string, error) {
	registered := make([]nodePlacementLoad, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == exclude || !n.Schedulable || n.Status != store.NodeStatusOnline {
			continue
		}
		registered = append(registered, nodePlacementLoad{NodeID: n.ID, Count: counts[n.ID]})
	}
	picked, err := p.pick(registered, c)
	if err == nil {
		return picked, nil
	}
	if exclude != "" && p.key(exclude) == "" {
		return "", err
	}
	localErr := p.fit("", c)
	if localErr == nil {
		return "", nil
	}
	var blocked *errNoGPUTarget
	if errors.As(err, &blocked) {
		return "", &errNoGPUTarget{reason: strings.TrimSuffix(blocked.reason, ")") + "; " + models.LocalNodeName + ": " + localErr.Error() + ")"}
	}
	return "", err
}

// gpuPlacementError explains why svc cannot move to nodeID, or returns ""
// when the move is fine. A node that has not reported a GPU snapshot yet
// is given the benefit of the doubt.
func (rt *Router) gpuPlacementError(ctx context.Context, svc *store.DesiredService, nodeID string) (string, error) {
	claim, ok := models.ServiceClaim(*svc)
	if !ok || rt.models == nil {
		return "", nil
	}
	_, known, err := rt.models.NodeGPU(ctx, nodeID)
	if err != nil {
		return "", err
	}
	if !known {
		return "", nil
	}
	planner, err := rt.newGPUPlanner(ctx)
	if err != nil {
		return "", err
	}
	err = planner.fit(nodeID, claim)
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, gpu.ErrNoGPU):
		return "this app requests a GPU (resources.gpu) and the target node has none", nil
	case errors.Is(err, gpu.ErrRuntimeMissing):
		return "the target node has a GPU but Docker has no nvidia runtime. " + gpu.InstallHint, nil
	}
	return "the target node cannot host this app's GPU request: " + err.Error(), nil
}

// settleGPUCreatePlacement checks (explicit node_id) or re-picks (auto
// placement) the node of a new GPU app. ok is false once the response
// has been written.
func (rt *Router) settleGPUCreatePlacement(w http.ResponseWriter, r *http.Request, body []byte, req *appResource) (ok bool) {
	claim, isGPU := models.ServiceClaim(store.DesiredService{Name: req.Name, Resources: req.Resources})
	if !isGPU || rt.models == nil {
		return true
	}
	if nodeIDKeyPresent(body) {
		msg, err := rt.gpuPlacementError(r.Context(), &store.DesiredService{Name: req.Name, Resources: req.Resources}, req.NodeID)
		if err != nil {
			rt.logger.Error("api: create app: gpu check failed", "error", err.Error(), "name", req.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return false
		}
		return true
	}
	planner, err := rt.newGPUPlanner(r.Context())
	if err != nil {
		rt.logger.Error("api: create app: gpu planner failed", "error", err.Error(), "name", req.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	if planner.fit(req.NodeID, claim) == nil {
		return true
	}
	target, err := rt.autoPlaceGPU(r.Context(), planner, claim)
	if err != nil {
		var blocked *errNoGPUTarget
		if errors.As(err, &blocked) {
			writeError(w, http.StatusConflict, blocked.Error()+". Add or fix a GPU node, or pass node_id explicitly.")
			return false
		}
		rt.logger.Error("api: create app: gpu auto-place failed", "error", err.Error(), "name", req.Name)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	req.NodeID = target
	req.AutoPlaced = target != ""
	return true
}

func (rt *Router) autoPlaceGPU(ctx context.Context, planner *gpuPlanner, claim gpu.Claim) (string, error) {
	if !rt.autoPlacementEnabled {
		if err := planner.fit("", claim); err != nil {
			return "", &errNoGPUTarget{reason: "no GPU node available (" + planner.label("") + ": " + err.Error() + ")"}
		}
		return "", nil
	}
	nodes, counts, err := rt.placementCounts(ctx)
	if err != nil {
		return "", err
	}
	return planner.spread(nodes, counts, "", claim)
}

// placementCounts returns the registered nodes and how many services and
// databases each hosts.
func (rt *Router) placementCounts(ctx context.Context) ([]store.Node, map[string]int, error) {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil, nil, err
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, nil, err
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, nil, err
	}
	counts := make(map[string]int, len(nodes))
	for _, s := range services {
		counts[s.NodeID]++
	}
	for _, d := range databases {
		counts[d.NodeID]++
	}
	return nodes, counts, nil
}

// doctorCheckGPUPlacement raises a warning per GPU app or model that no
// node can host, which the CLI and dashboard surface as attention items.
func (rt *Router) doctorCheckGPUPlacement(ctx context.Context) []doctorCheckResource {
	if rt.models == nil {
		return nil
	}
	unplaced, err := rt.models.UnplacedWorkloads(ctx)
	if err != nil {
		return nil
	}
	out := make([]doctorCheckResource, 0, len(unplaced))
	for _, u := range unplaced {
		out = append(out, doctorCheckResource{
			Code:     "gpu-placement:" + u.Kind + ":" + u.Name,
			Name:     fmt.Sprintf("GPU %s %s cannot be placed", u.Kind, u.Name),
			Status:   doctorStatusWarn,
			Message:  "no GPU node has enough free GPUs: " + u.Reason,
			Fix:      "Free a GPU (stop or shrink another GPU workload), add a GPU node, or install the nvidia container toolkit on the node. See `levelrail models gpus`.",
			DocsPath: "/ai-models#gpu-scheduling",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// nodeGPUResource is the GPU summary carried on the node list and detail.
type nodeGPUResource struct {
	Present          bool     `json:"present"`
	RuntimeInstalled bool     `json:"runtime_installed"`
	DriverVersion    string   `json:"driver_version,omitempty"`
	GPUCount         int      `json:"gpu_count"`
	ReservedGPUs     int      `json:"reserved_gpus"`
	FreeGPUs         int      `json:"free_gpus"`
	TotalVRAMMiB     int64    `json:"total_vram_mib"`
	UsedVRAMMiB      int64    `json:"used_vram_mib"`
	Reservations     []string `json:"reservations"`
}

// nodeGPUResources maps node ID to its GPU summary, omitting nodes with
// no GPU. Empty when models are not configured.
func (rt *Router) nodeGPUResources(ctx context.Context) map[string]*nodeGPUResource {
	out := map[string]*nodeGPUResource{}
	if rt.models == nil {
		return out
	}
	nodes, err := rt.models.GPUNodes(ctx)
	if err != nil {
		rt.logger.Error("api: node gpu summary failed", "error", err.Error())
		return out
	}
	for _, n := range nodes {
		if !n.Info.Present {
			continue
		}
		g := toGPUNodeResource(n)
		out[n.NodeID] = &nodeGPUResource{
			Present: g.Present, RuntimeInstalled: g.RuntimeInstalled, DriverVersion: g.DriverVersion, GPUCount: g.GPUCount,
			ReservedGPUs: g.ReservedGPUs, FreeGPUs: g.FreeGPUs, TotalVRAMMiB: g.TotalVRAMMiB, UsedVRAMMiB: g.UsedVRAMMiB,
			Reservations: g.Reservations,
		}
	}
	return out
}
