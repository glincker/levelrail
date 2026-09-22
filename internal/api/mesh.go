package api

// This file: the WireGuard mesh status and key-rotation routes.
//
// A real, honest limitation runs through every handler here, stated once
// rather than repeated at every call site: internal/network.ConfigSink's
// gRPC arm (the piece that would carry a config, a status query, or a
// rotation request to a *remote* node over the agent Session stream)
// does not exist yet (see that interface's own doc comment, and
// cmd/levelrail/mesh.go's top-of-file note on why APP_MESH_ENABLED is
// still opt-in). What exists today is one Coordinator and one Mesh
// device, both for the control plane's own node.
// So:
//
//   - GET /api/v1/mesh returns real, UAPI-backed peer and handshake data
//     for that one node, plus best-effort static info (public key, mesh
//     address, as last persisted by the mesh reconciler) for every other
//     node in the fleet, clearly labeled as such rather than presented
//     as equally live.
//   - POST /api/v1/nodes/{id}/mesh/rotate-key only actually succeeds for
//     the local node; asking it to rotate any other node's key returns a
//     clear 501 naming the same gap, not a silent no-op or a misleading
//     success.
//
// Both routes degrade to 501 entirely when mesh networking was never
// enabled (APP_MESH_ENABLED unset), the same "not configured, not
// broken" shape WithSecretSetter's own absence already uses elsewhere in
// this package.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MeshStatusProvider is the narrow surface this package needs from this
// node's live network.Mesh: exactly Status, never Apply or Close, since
// this package only ever reads the mesh, never drives it (the reconcile
// loop owns that). *network.Device, *network.Disabled, and any test fake
// implementing Status alone all satisfy this.
type MeshStatusProvider interface {
	Status(ctx context.Context) (network.Status, error)
}

// MeshKeyRotator is the narrow surface this package needs from
// *network.Coordinator for rotation: rotate one node's key, and read
// back what Coordinator itself knows about in-flight rotations.
// *network.Coordinator satisfies this structurally.
type MeshKeyRotator interface {
	RotateKey(ctx context.Context, nodeID string) (network.RotationResult, error)
	RotationStatusFor(nodeID string) (network.RotationStatus, bool)
}

// meshStaleAfter mirrors internal/network's own unexported staleAfter
// (coordinator.go): three minutes, one missed WireGuard rekey plus
// margin. Duplicated rather than exported from internal/network
// specifically so that package's own health threshold stays a decision
// that package owns; this is only the wire-response mirror of it.
const meshStaleAfter = 3 * time.Minute

// meshPeerResource is one peer as this node's device sees it, joined
// with whatever the store knows about that peer's identity.
type meshPeerResource struct {
	NodeID        string     `json:"node_id"`
	Name          string     `json:"name,omitempty"`
	PublicKey     string     `json:"public_key"`
	MeshAddress   string     `json:"mesh_address,omitempty"`
	Endpoint      string     `json:"endpoint,omitempty"`
	LastHandshake *time.Time `json:"last_handshake_at,omitempty"`
	Healthy       bool       `json:"healthy"`
	TransferRx    int64      `json:"transfer_rx_bytes"`
	TransferTx    int64      `json:"transfer_tx_bytes"`
	// Live is false for a node this control plane cannot query the UAPI
	// of directly (every node but its own): PublicKey/MeshAddress still
	// come from the store, but Endpoint/LastHandshake/Healthy/Transfer
	// are this node's own device's view of that peer instead, which is
	// live and real, just gathered from the local side of the
	// connection rather than the remote one. See this file's own header.
	Live bool `json:"live"`
}

// meshRotationResource is a network.RotationStatus's wire shape.
type meshRotationResource struct {
	NodeID       string     `json:"node_id"`
	OldPublicKey string     `json:"old_public_key"`
	NewPublicKey string     `json:"new_public_key"`
	StartedAt    time.Time  `json:"started_at"`
	Confirmed    bool       `json:"confirmed"`
	ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
}

func toMeshRotationResource(s network.RotationStatus) meshRotationResource {
	res := meshRotationResource{
		NodeID:       s.NodeID,
		OldPublicKey: s.OldPublicKey.String(),
		NewPublicKey: s.NewPublicKey.String(),
		StartedAt:    s.StartedAt,
		Confirmed:    s.Confirmed,
	}
	if s.Confirmed {
		t := s.ConfirmedAt
		res.ConfirmedAt = &t
	}
	return res
}

// meshStatusResource is GET /api/v1/mesh's response body.
type meshStatusResource struct {
	Enabled     bool               `json:"enabled"`
	Backend     string             `json:"backend,omitempty"`
	Interface   string             `json:"interface,omitempty"`
	LocalNodeID string             `json:"local_node_id,omitempty"`
	PublicKey   string             `json:"public_key,omitempty"`
	MeshAddress string             `json:"mesh_address,omitempty"`
	ListenPort  int                `json:"listen_port,omitempty"`
	Peers       []meshPeerResource `json:"peers"`
	// Rotation is the local node's own most recent rotation, if any has
	// happened since this process started. Absent when none has.
	Rotation *meshRotationResource `json:"rotation,omitempty"`
}

// handleGetMeshStatus handles GET /api/v1/mesh: this node's live mesh
// state, every peer it currently knows about, and this node's own most
// recent key rotation (if any). Returns 501 when mesh networking is not
// enabled on this control plane at all.
func (rt *Router) handleGetMeshStatus(w http.ResponseWriter, r *http.Request) {
	if rt.mesh == nil {
		writeError(w, http.StatusNotImplemented, "mesh networking is not enabled on this control plane")
		return
	}

	st, err := rt.mesh.Status(r.Context())
	if err != nil {
		rt.logger.Error("api: get mesh status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.logger.Error("api: get mesh status: list nodes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	res := meshStatusResource{
		Enabled:     true,
		Backend:     string(st.Backend),
		Interface:   st.Interface,
		LocalNodeID: rt.localNodeID,
		PublicKey:   st.PublicKey.String(),
		ListenPort:  st.ListenPort,
		Peers:       toMeshPeerResources(st, nodes, time.Now()),
	}
	if st.Address.IsValid() {
		res.MeshAddress = st.Address.String()
	}
	if rt.meshRotator != nil {
		if status, ok := rt.meshRotator.RotationStatusFor(rt.localNodeID); ok {
			wire := toMeshRotationResource(status)
			res.Rotation = &wire
		}
	}

	writeJSON(w, http.StatusOK, res)
}

// toMeshPeerResources joins the live device status with the store's
// record of the fleet: every live peer entry from st.Peers is matched to
// its node row by public key for a name and any store-recorded mesh
// address, and every *other* node the store knows about (one this node
// has not yet peered with, or lost its live entry for) is still listed,
// with Live: false and no handshake data, rather than silently omitted.
// An operator debugging "why can't service A reach node B" needs to see
// B in this list even when B has never once handshaked, which is the
// whole point of a status view: it must show failure, not just success.
func toMeshPeerResources(st network.Status, nodes []store.Node, now time.Time) []meshPeerResource {
	byPubKey := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		if n.MeshPublicKey != "" {
			byPubKey[n.MeshPublicKey] = n
		}
	}

	seen := make(map[string]struct{}, len(st.Peers))
	out := make([]meshPeerResource, 0, len(nodes))

	for _, p := range st.Peers {
		res := meshPeerResource{
			NodeID:     p.NodeID,
			PublicKey:  p.PublicKey.String(),
			Endpoint:   p.Endpoint,
			Healthy:    p.Healthy(now, meshStaleAfter),
			TransferRx: p.TransferRx,
			TransferTx: p.TransferTx,
			Live:       true,
		}
		if !p.LastHandshake.IsZero() {
			t := p.LastHandshake
			res.LastHandshake = &t
		}
		if n, ok := byPubKey[p.PublicKey.String()]; ok {
			res.Name = n.Name
			res.MeshAddress = n.MeshAddress
			if res.NodeID == "" {
				res.NodeID = n.ID
			}
			seen[n.ID] = struct{}{}
		}
		out = append(out, res)
	}

	// Nodes the local device has no live peer entry for at all: never
	// configured as a peer yet (mesh controller hasn't reached this pass
	// for them), or this is the local node's own row.
	for _, n := range nodes {
		if _, already := seen[n.ID]; already {
			continue
		}
		if n.MeshPublicKey == "" {
			continue
		}
		out = append(out, meshPeerResource{
			NodeID:      n.ID,
			Name:        n.Name,
			PublicKey:   n.MeshPublicKey,
			MeshAddress: n.MeshAddress,
			Live:        false,
		})
	}
	return out
}

// rotateKeyResponse is POST .../mesh/rotate-key's response body.
type rotateKeyResponse struct {
	NodeID       string `json:"node_id"`
	OldPublicKey string `json:"old_public_key"`
	NewPublicKey string `json:"new_public_key"`
}

// handleRotateNodeMeshKey handles POST /api/v1/nodes/{id}/mesh/rotate-key:
// generates a fresh WireGuard keypair for id, makes it that node's live
// identity immediately, and returns the change. See this file's own
// header for what "immediately" means for partition safety (a brief
// reconnect blip is possible while the rest of the fleet's next reconcile
// pass catches up, tracked via the rotation field on
// GET /api/v1/mesh), and for why this only actually works for id ==
// this control plane's own node today.
func (rt *Router) handleRotateNodeMeshKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if rt.meshRotator == nil {
		writeError(w, http.StatusNotImplemented, "mesh networking is not enabled on this control plane")
		return
	}
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: rotate node mesh key: look up node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result, err := rt.meshRotator.RotateKey(r.Context(), id)
	switch {
	case errors.Is(err, network.ErrRotationNotSupported), errors.Is(err, network.ErrUnknownNode):
		writeError(w, http.StatusNotImplemented,
			"key rotation is only available for the node running this control plane; remote-node rotation needs the agent wire extension, which does not exist yet")
		return
	case err != nil:
		rt.logger.Error("api: rotate node mesh key failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if rt.reconcileNudger != nil {
		// The rotated key is already live on the device (LocalSink.RotateKey's
		// own doc comment), but every *other* node still has the old one
		// in its peer list until the next mesh reconcile pass sends the
		// new one. Nudging here is what makes GET /api/v1/mesh's
		// "rotation.confirmed" flip promptly instead of waiting out
		// whatever the resync interval happens to be, the same reasoning
		// every other desired-state-changing handler in this package
		// already nudges for.
		rt.reconcileNudger.Nudge()
	}

	writeJSON(w, http.StatusOK, rotateKeyResponse{
		NodeID:       result.NodeID,
		OldPublicKey: result.OldPublicKey.String(),
		NewPublicKey: result.NewPublicKey.String(),
	})
}
