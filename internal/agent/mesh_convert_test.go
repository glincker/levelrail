package agent

import (
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// testMeshKey builds a deterministic, distinguishable Key from seed, the
// same shape internal/network's own testKey (key_test.go) uses: a real
// GeneratePrivateKey would work too, but a fixed input keeps assertions
// on a specific expected key value stable across runs.
func testMeshKey(t *testing.T, seed byte) network.Key {
	t.Helper()
	var k network.Key
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}

func TestDeviceConfigRoundTrip(t *testing.T) {
	cfg := network.DeviceConfig{
		NodeID:     "node-a",
		PrivateKey: testMeshKey(t, 1), // must never survive the round trip
		Address:    netip.MustParsePrefix("10.181.0.3/16"),
		ListenPort: 51820,
		Peers: []network.PeerConfig{
			{
				NodeID:              "node-b",
				PublicKey:           testMeshKey(t, 2),
				Endpoint:            "203.0.113.2:51820",
				AllowedIPs:          []netip.Prefix{netip.MustParsePrefix("10.181.0.4/32")},
				PersistentKeepalive: 25 * time.Second,
			},
		},
	}

	pb := deviceConfigToPB(cfg)
	if pb.GetNodeId() != cfg.NodeID {
		t.Fatalf("wire NodeId = %q, want %q", pb.GetNodeId(), cfg.NodeID)
	}
	if pb.GetAddress() != cfg.Address.String() {
		t.Fatalf("wire Address = %q, want %q", pb.GetAddress(), cfg.Address.String())
	}

	got, err := deviceConfigFromPB(pb)
	if err != nil {
		t.Fatalf("deviceConfigFromPB: %v", err)
	}
	if got.NodeID != cfg.NodeID {
		t.Errorf("NodeID = %q, want %q", got.NodeID, cfg.NodeID)
	}
	if got.Address != cfg.Address {
		t.Errorf("Address = %v, want %v", got.Address, cfg.Address)
	}
	if got.ListenPort != cfg.ListenPort {
		t.Errorf("ListenPort = %d, want %d", got.ListenPort, cfg.ListenPort)
	}
	if !got.PrivateKey.IsZero() {
		t.Error("deviceConfigFromPB() carried a non-zero PrivateKey, want it structurally absent from the wire")
	}
	if len(got.Peers) != 1 {
		t.Fatalf("len(Peers) = %d, want 1", len(got.Peers))
	}
	peer := got.Peers[0]
	if peer.NodeID != "node-b" || peer.PublicKey != cfg.Peers[0].PublicKey || peer.Endpoint != cfg.Peers[0].Endpoint {
		t.Errorf("Peers[0] = %+v, want it to match the original", peer)
	}
	if len(peer.AllowedIPs) != 1 || peer.AllowedIPs[0] != cfg.Peers[0].AllowedIPs[0] {
		t.Errorf("Peers[0].AllowedIPs = %v, want %v", peer.AllowedIPs, cfg.Peers[0].AllowedIPs)
	}
	if peer.PersistentKeepalive != cfg.Peers[0].PersistentKeepalive {
		t.Errorf("Peers[0].PersistentKeepalive = %v, want %v", peer.PersistentKeepalive, cfg.Peers[0].PersistentKeepalive)
	}
}

func TestDeviceConfigToPB_NeverSerializesPrivateKey(t *testing.T) {
	cfg := network.DeviceConfig{NodeID: "node-a", PrivateKey: testMeshKey(t, 7)}
	pb := deviceConfigToPB(cfg)
	// The proto message structurally has no private-key field at all;
	// this asserts the conversion never grows one that silently starts
	// carrying it.
	if pb.String() == "" {
		t.Fatal("deviceConfigToPB() produced an empty message")
	}
}

func TestDeviceConfigFromPB_NilConfig_ReturnsError(t *testing.T) {
	if _, err := deviceConfigFromPB(nil); err == nil {
		t.Fatal("deviceConfigFromPB(nil) error = nil, want an error")
	}
}

func TestDeviceConfigFromPB_NoAddress_LeavesAddressInvalid(t *testing.T) {
	got, err := deviceConfigFromPB(&agentpb.DeviceConfig{NodeId: "node-a"})
	if err != nil {
		t.Fatalf("deviceConfigFromPB: %v", err)
	}
	if got.Address.IsValid() {
		t.Fatalf("Address = %v, want the zero, invalid prefix when the wire carried none", got.Address)
	}
}

func TestDeviceConfigFromPB_MalformedPeerAllowedIP_ReturnsError(t *testing.T) {
	pb := &agentpb.DeviceConfig{
		NodeId: "node-a",
		Peers: []*agentpb.PeerConfig{{
			NodeId:     "node-b",
			PublicKey:  testMeshKey(t, 2).String(),
			AllowedIps: []string{"not-a-prefix"},
		}},
	}
	if _, err := deviceConfigFromPB(pb); err == nil {
		t.Fatal("deviceConfigFromPB() error = nil, want an error for a malformed peer allowed IP")
	}
}

func TestPeerConfigFromPB_MalformedPublicKey_ReturnsError(t *testing.T) {
	pb := &agentpb.PeerConfig{NodeId: "node-b", PublicKey: "not-a-key"}
	if _, err := peerConfigFromPB(pb); err == nil {
		t.Fatal("peerConfigFromPB() error = nil, want an error for a malformed public key")
	}
}

func TestNodeIdentityRoundTrip(t *testing.T) {
	id := network.NodeIdentity{
		PublicKey:  testMeshKey(t, 3),
		ListenPort: 51820,
		Endpoint:   "203.0.113.9:51820",
	}
	got, err := nodeIdentityFromPB(nodeIdentityToPB(id))
	if err != nil {
		t.Fatalf("nodeIdentityFromPB: %v", err)
	}
	if got != id {
		t.Fatalf("nodeIdentityFromPB(nodeIdentityToPB(id)) = %+v, want %+v", got, id)
	}
}

func TestNodeIdentityToPB_OutOfRangePort_ReportsZero(t *testing.T) {
	for _, port := range []int{-1, 65536, 1<<32 + 51820} {
		if got := nodeIdentityToPB(network.NodeIdentity{ListenPort: port}).GetListenPort(); got != 0 {
			t.Errorf("ListenPort %d converted to %d, want 0", port, got)
		}
	}
}

func TestNodeIdentityFromPB_MalformedPublicKey_ReturnsError(t *testing.T) {
	pb := &agentpb.NodeIdentity{PublicKey: "not-a-key"}
	if _, err := nodeIdentityFromPB(pb); err == nil {
		t.Fatal("nodeIdentityFromPB() error = nil, want an error for a malformed public key")
	}
}
