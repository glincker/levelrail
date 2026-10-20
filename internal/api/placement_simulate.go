package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxSimulatedApps  = 200
	simulationMiBToBy = 1 << 20
)

type placementSimulationResponse struct {
	Request placementSimulationRequest `json:"request"`
	placement.Simulation
}

type placementSimulationRequest struct {
	CPUMilli  int64  `json:"cpu_milli"`
	MemoryMiB int64  `json:"memory_mib"`
	Count     int    `json:"count"`
	Selector  string `json:"selector,omitempty"`
}

// handlePlacementSimulation handles GET /api/v1/nodes/placement-simulation:
// where count identical apps would land right now, nothing is created.
func (rt *Router) handlePlacementSimulation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cpu, err1 := queryInt(q.Get("cpu_milli"), 0)
	mem, err2 := queryInt(q.Get("memory_mib"), 0)
	count, err3 := queryInt(q.Get("count"), 1)
	if err1 != nil || err2 != nil || err3 != nil || cpu < 0 || mem < 0 || count < 1 || count > maxSimulatedApps {
		writeError(w, http.StatusBadRequest, "cpu_milli and memory_mib must be non-negative integers and count between 1 and "+strconv.Itoa(maxSimulatedApps))
		return
	}
	var sel placement.Selector
	if raw := q.Get("selector"); raw != "" {
		labels, err := placement.ParseLabels(strings.Split(raw, ","))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sel = labels
	}
	cands, err := rt.placementCandidates(r.Context())
	if err != nil {
		rt.internalError(w, "api: placement simulation: candidates failed", err)
		return
	}
	req := placement.Request{
		Selector: sel, Policy: rt.placementPolicy,
		Needs: placement.Resources{CPUMilli: int64(cpu), MemoryBytes: int64(mem) * simulationMiBToBy},
	}
	writeJSON(w, http.StatusOK, placementSimulationResponse{
		Request:    placementSimulationRequest{CPUMilli: int64(cpu), MemoryMiB: int64(mem), Count: count, Selector: sel.String()},
		Simulation: placement.Simulate(cands, req, count),
	})
}

type failureSimulationResponse struct {
	Zone  string   `json:"zone,omitempty"`
	Node  string   `json:"node,omitempty"`
	Lost  []string `json:"lost_node_ids"`
	Count int      `json:"displaced"`
	placement.Simulation
}

// handleFailureSimulation handles GET /api/v1/nodes/failure-simulation: what
// the scheduler would do if a zone (or one node) vanished right now.
func (rt *Router) handleFailureSimulation(w http.ResponseWriter, r *http.Request) {
	zone, node := r.URL.Query().Get("zone"), r.URL.Query().Get("node")
	if (zone == "") == (node == "") {
		writeError(w, http.StatusBadRequest, "pass exactly one of zone or node")
		return
	}
	inv, err := rt.loadPlacementInventory(r.Context())
	if err != nil {
		rt.internalError(w, "api: failure simulation: load failed", err)
		return
	}
	cands, err := rt.candidatesFrom(r.Context(), inv)
	if err != nil {
		rt.internalError(w, "api: failure simulation: candidates failed", err)
		return
	}
	dead := func(c placement.Candidate) bool {
		return c.ID == node || (zone != "" && placement.ZoneOf(c.Labels) == zone)
	}
	out := failureSimulationResponse{Zone: zone, Node: node, Lost: []string{}}
	for _, c := range cands {
		if dead(c) {
			out.Lost = append(out.Lost, c.ID)
		}
	}
	if len(out.Lost) == 0 {
		writeError(w, http.StatusNotFound, "no node matches that zone or node id")
		return
	}
	work, err := rt.displaceableWorkloads(r)
	if err != nil {
		rt.internalError(w, "api: failure simulation: workloads failed", err)
		return
	}
	out.Simulation = placement.FailNodes(cands, work, dead)
	out.Count = len(out.Placed)
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) displaceableWorkloads(r *http.Request) ([]placement.Workload, error) {
	ctx := r.Context()
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	home := func(id string) string {
		if id == "" {
			return rt.localNodeID
		}
		return id
	}
	var out []placement.Workload
	for _, s := range services {
		sel, err := rt.selectorFor(ctx, store.NodeMoveKindApp, s.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, placement.Workload{Name: s.Name, NodeID: home(s.NodeID), Needs: serviceRequests(s.Resources, s.Replicas), Selector: sel})
	}
	for _, d := range databases {
		sel, err := rt.selectorFor(ctx, store.NodeMoveKindDatabase, d.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, placement.Workload{Name: d.Name, NodeID: home(d.NodeID), Selector: sel})
	}
	return out, nil
}

func queryInt(raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	return strconv.Atoi(raw)
}
