package agent

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// MeshStatusReader is the optional half of a MeshApplier that can report
// the node's live WireGuard view, for node-pair diagnostics.
type MeshStatusReader interface {
	Status(ctx context.Context) (network.Status, error)
}

func handleMeshStatus(ctx context.Context, mesh MeshApplier, requestID string, send func(*agentpb.AgentMessage)) {
	resp := &agentpb.AgentResponse{RequestId: requestID}
	defer func() { send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}}) }()

	reader, ok := mesh.(MeshStatusReader)
	if mesh == nil || !ok {
		resp.Error = ErrMeshUnavailable.Error()
		return
	}
	st, err := reader.Status(ctx)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Result = &agentpb.AgentResponse_MeshStatus{MeshStatus: meshStatusToPB(st)}
}

func meshStatusToPB(st network.Status) *agentpb.MeshStatusResponse {
	out := &agentpb.MeshStatusResponse{}
	if st.Address.IsValid() {
		out.Address = st.Address.String()
	}
	for _, p := range st.Peers {
		var hs int64
		if !p.LastHandshake.IsZero() {
			hs = p.LastHandshake.UnixMilli()
		}
		out.Peers = append(out.Peers, &agentpb.MeshPeerStatus{
			NodeId: p.NodeID, PublicKey: p.PublicKey.String(), Endpoint: p.Endpoint,
			LastHandshakeUnixMs: hs, RxBytes: p.TransferRx, TxBytes: p.TransferTx,
		})
	}
	return out
}

func meshStatusFromPB(r *agentpb.MeshStatusResponse) (network.Status, error) {
	var st network.Status
	if a := r.GetAddress(); a != "" {
		prefix, err := netip.ParsePrefix(a)
		if err != nil {
			return network.Status{}, fmt.Errorf("agent: mesh status: parse address %q: %w", a, err)
		}
		st.Address = prefix
	}
	for _, p := range r.GetPeers() {
		key, err := network.ParseKey(p.GetPublicKey())
		if err != nil {
			return network.Status{}, fmt.Errorf("agent: mesh status: parse peer key: %w", err)
		}
		ps := network.PeerStatus{NodeID: p.GetNodeId(), PublicKey: key, Endpoint: p.GetEndpoint(), TransferRx: p.GetRxBytes(), TransferTx: p.GetTxBytes()}
		if ms := p.GetLastHandshakeUnixMs(); ms > 0 {
			ps.LastHandshake = time.UnixMilli(ms)
		}
		st.Peers = append(st.Peers, ps)
	}
	return st, nil
}

// MeshStatus asks this transport's node for its live WireGuard view.
func (t *GRPCTransport) MeshStatus(ctx context.Context) (network.Status, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_MeshStatus{MeshStatus: &agentpb.MeshStatusRequest{}},
	})
	if err != nil {
		return network.Status{}, err
	}
	return meshStatusFromPB(resp.GetMeshStatus())
}

type meshStatusTransport interface {
	MeshStatus(ctx context.Context) (network.Status, error)
}

// MeshStatus returns nodeID's live WireGuard view over its agent session.
func (s *GRPCSink) MeshStatus(ctx context.Context, nodeID string) (network.Status, error) {
	t, err := s.registry.Get(nodeID)
	if err != nil {
		return network.Status{}, err
	}
	mt, ok := t.(meshStatusTransport)
	if !ok {
		return network.Status{}, fmt.Errorf("%w: node %q", ErrNodeNotMeshCapable, nodeID)
	}
	return mt.MeshStatus(ctx)
}
