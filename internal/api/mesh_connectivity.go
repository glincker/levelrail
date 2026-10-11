package api

import (
	"context"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Per-direction and per-pair link states for GET /api/v1/mesh/connectivity.
const (
	linkHealthy = "healthy"
	linkStale   = "stale"
	linkNever   = "never"
	linkNoPeer  = "no_peer"
	linkUnknown = "unknown"

	pairOK       = "ok"
	pairDegraded = "degraded"
	pairDown     = "down"
	pairUnknown  = "unknown"
)

type meshLinkView struct {
	State         string     `json:"state"`
	LastHandshake *time.Time `json:"last_handshake,omitempty"`
	Endpoint      string     `json:"endpoint,omitempty"`
}

type meshPairResource struct {
	A      string       `json:"a"`
	B      string       `json:"b"`
	Status string       `json:"status"`
	AToB   meshLinkView `json:"a_to_b"`
	BToA   meshLinkView `json:"b_to_a"`
	Hint   string       `json:"hint,omitempty"`
}

type meshNodeView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MeshAddress string `json:"mesh_address"`
	ViewError   string `json:"view_error,omitempty"`
}

type meshConnectivityResource struct {
	Nodes     []meshNodeView     `json:"nodes"`
	Pairs     []meshPairResource `json:"pairs"`
	CheckedAt time.Time          `json:"checked_at"`
}

// meshViewTimeout bounds each node's status call so one hung agent cannot stall the matrix.
const meshViewTimeout = 5 * time.Second

// handleMeshConnectivity handles GET /api/v1/mesh/connectivity: every
// node's own WireGuard view of every other node, as a pair matrix.
func (rt *Router) handleMeshConnectivity(w http.ResponseWriter, r *http.Request) {
	if rt.mesh == nil {
		writeError(w, http.StatusNotImplemented, "mesh networking is not enabled on this control plane")
		return
	}
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.internalError(w, "api: mesh connectivity: list nodes failed", err)
		return
	}
	var members []store.Node
	for _, n := range nodes {
		if n.MeshPublicKey != "" {
			members = append(members, n)
		}
	}
	views := make(map[string]network.Status, len(members))
	out := meshConnectivityResource{Nodes: []meshNodeView{}, Pairs: []meshPairResource{}, CheckedAt: time.Now().UTC()}
	for _, n := range members {
		st, err := rt.meshViewOf(r.Context(), n.ID)
		v := meshNodeView{ID: n.ID, Name: n.Name, MeshAddress: n.MeshAddress}
		if err != nil {
			v.ViewError = err.Error()
		} else {
			views[n.ID] = st
		}
		out.Nodes = append(out.Nodes, v)
	}
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			a, b := members[i], members[j]
			pair := meshPairResource{A: a.ID, B: b.ID, AToB: linkView(views, a.ID, b, out.CheckedAt), BToA: linkView(views, b.ID, a, out.CheckedAt)}
			pair.Status, pair.Hint = pairStatus(pair.AToB, pair.BToA)
			out.Pairs = append(out.Pairs, pair)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) meshViewOf(ctx context.Context, nodeID string) (network.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, meshViewTimeout)
	defer cancel()
	if nodeID == rt.localNodeID {
		return rt.mesh.Status(ctx)
	}
	if rt.multiNode == nil || rt.multiNode.peerStatus == nil {
		return network.Status{}, errNoPeerStatus
	}
	return rt.multiNode.peerStatus.MeshStatus(ctx, nodeID)
}

type meshViewError string

func (e meshViewError) Error() string { return string(e) }

const errNoPeerStatus = meshViewError("remote mesh status is not wired on this control plane")

func linkView(views map[string]network.Status, from string, to store.Node, now time.Time) meshLinkView {
	st, ok := views[from]
	if !ok {
		return meshLinkView{State: linkUnknown}
	}
	for _, p := range st.Peers {
		if p.PublicKey.String() != to.MeshPublicKey {
			continue
		}
		v := meshLinkView{Endpoint: p.Endpoint}
		switch {
		case p.LastHandshake.IsZero():
			v.State = linkNever
		case p.Healthy(now, meshpath.StaleAfter):
			v.State = linkHealthy
		default:
			v.State = linkStale
		}
		if !p.LastHandshake.IsZero() {
			hs := p.LastHandshake.UTC()
			v.LastHandshake = &hs
		}
		return v
	}
	return meshLinkView{State: linkNoPeer}
}

func pairStatus(ab, ba meshLinkView) (string, string) {
	switch {
	case ab.State == linkHealthy && ba.State == linkHealthy:
		return pairOK, ""
	case ab.State == linkUnknown && ba.State == linkUnknown:
		return pairUnknown, "neither node reported its mesh view: check both agents are connected"
	case ab.State == linkNoPeer || ba.State == linkNoPeer:
		return pairDown, "a node has no peer entry for the other yet: run `nodes rejoin-mesh` or wait for the mesh reconciler"
	case ab.State == linkHealthy || ba.State == linkHealthy:
		return pairDegraded, "only one side sees a recent handshake: check UDP 51820 is open in both directions"
	default:
		return pairDown, "no recent handshake either way: open UDP 51820 between the two hosts and check each node's public endpoint"
	}
}
