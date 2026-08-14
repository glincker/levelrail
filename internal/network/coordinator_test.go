package network

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

// recordingSink is a ConfigSink that answers with a canned identity per
// node and records what it was sent. The identity is what a real node
// would report after applying: its own public key, derived from a private
// key the control plane never sees.
type recordingSink struct {
	identities map[string]NodeIdentity
	failures   map[string]error
	received   map[string]DeviceConfig
	order      []string
}

func newRecordingSink() *recordingSink {
	return &recordingSink{
		identities: map[string]NodeIdentity{},
		failures:   map[string]error{},
		received:   map[string]DeviceConfig{},
	}
}

func (s *recordingSink) ApplyMesh(_ context.Context, nodeID string, cfg DeviceConfig) (NodeIdentity, error) {
	s.order = append(s.order, nodeID)
	if err, bad := s.failures[nodeID]; bad {
		return NodeIdentity{}, err
	}
	s.received[nodeID] = cfg
	return s.identities[nodeID], nil
}

func TestCoordinator_DistributesAFullMesh(t *testing.T) {
	sink := newRecordingSink()
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(1), ListenPort: 51820}
	sink.identities["b"] = NodeIdentity{PublicKey: testKey(2), ListenPort: 51820}
	sink.identities["c"] = NodeIdentity{PublicKey: testKey(3), ListenPort: 51820}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))

	inventory := []NodeInfo{
		node("a", 1, "10.181.0.1", "203.0.113.1:51820"),
		node("b", 2, "10.181.0.2", "203.0.113.2:51820"),
		node("c", 3, "10.181.0.3", "203.0.113.3:51820"),
	}

	_, result, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}
	if len(result.Failed()) != 0 {
		t.Fatalf("unexpected failures: %+v", result.Failed())
	}
	if len(result.Nodes) != 3 {
		t.Fatalf("got %d node results, want 3", len(result.Nodes))
	}

	for id, cfg := range sink.received {
		if len(cfg.Peers) != 2 {
			t.Errorf("node %q was sent %d peers, want 2", id, len(cfg.Peers))
		}
		if !cfg.PrivateKey.IsZero() {
			t.Errorf("node %q was sent a private key: the control plane must never hold or ship one", id)
		}
		if _, self := cfg.PeerByNodeID(id); self {
			t.Errorf("node %q was sent itself as a peer", id)
		}
	}
}

// TestCoordinator_ConvergesOverTwoPasses is the concrete consequence of
// the key-exchange design: a brand new node cannot be fully meshed in one
// pass, because its public key only exists after it applies its first
// config. The property to prove is that pass two closes the gap.
func TestCoordinator_ConvergesOverTwoPasses(t *testing.T) {
	sink := newRecordingSink()
	sink.identities["existing"] = NodeIdentity{PublicKey: testKey(1)}
	sink.identities["fresh"] = NodeIdentity{PublicKey: testKey(2)}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))

	inventory := []NodeInfo{
		node("existing", 1, "10.181.0.1", ""),
		{ID: "fresh", Name: "fresh"}, // enrolled, no key, no address yet
	}

	inventory, first, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("first Distribute: %v", err)
	}
	if len(first.Updated) != 1 || first.Updated[0] != "fresh" {
		t.Fatalf("Updated = %v, want [fresh]: the new node reported a key the inventory did not have", first.Updated)
	}
	// Pass one: the established node does not know about the new one yet,
	// because the new one had not reported a key when the plan was made.
	if peers := len(sink.received["existing"].Peers); peers != 0 {
		t.Errorf("pass one: node existing got %d peers, want 0", peers)
	}
	// The new node was still given an address and its config, which is
	// what lets it produce a key at all.
	if !sink.received["fresh"].Address.IsValid() {
		t.Error("pass one: the new node was not given a mesh address")
	}

	_, second, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("second Distribute: %v", err)
	}
	if len(second.Updated) != 0 {
		t.Errorf("pass two Updated = %v, want empty: the mesh has converged", second.Updated)
	}
	if peers := len(sink.received["existing"].Peers); peers != 1 {
		t.Fatalf("pass two: node existing got %d peers, want 1", peers)
	}
	if p, ok := sink.received["existing"].PeerByNodeID("fresh"); !ok || p.PublicKey != testKey(2) {
		t.Error("pass two: the established node did not learn the new node's key")
	}
}

// TestCoordinator_UnreachableNodeDoesNotBlockTheFleet is the failure mode
// that matters most: the node most likely to be unreachable is the one
// that just went down, which is exactly when the others most need an
// updated peer list.
func TestCoordinator_UnreachableNodeDoesNotBlockTheFleet(t *testing.T) {
	sink := newRecordingSink()
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(1)}
	sink.identities["c"] = NodeIdentity{PublicKey: testKey(3)}
	sink.failures["b"] = errors.New("node not registered in this transport registry")

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	inventory := []NodeInfo{
		node("a", 1, "10.181.0.1", ""),
		node("b", 2, "10.181.0.2", ""),
		node("c", 3, "10.181.0.3", ""),
	}

	_, result, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("Distribute returned an error for one unreachable node: %v", err)
	}

	failed := result.Failed()
	if len(failed) != 1 || failed[0].NodeID != "b" {
		t.Fatalf("Failed() = %+v, want exactly node b", failed)
	}
	if !strings.Contains(failed[0].Err.Error(), `node "b"`) {
		t.Errorf("failure error = %q, want it to name the node", failed[0].Err)
	}
	if _, reached := sink.received["a"]; !reached {
		t.Error("node a was not reached because node b failed")
	}
	if _, reached := sink.received["c"]; !reached {
		t.Error("node c was not reached because node b failed")
	}
}

func TestCoordinator_ObservedEndpointBeatsSelfReported(t *testing.T) {
	sink := newRecordingSink()
	// What a NATted node believes about itself: its private address,
	// which no other node can reach.
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(1), Endpoint: "192.168.1.50:51820"}
	sink.identities["b"] = NodeIdentity{PublicKey: testKey(2)}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	if err := c.SetObservedEndpoint("a", "203.0.113.9:51820"); err != nil {
		t.Fatalf("SetObservedEndpoint: %v", err)
	}

	inventory := []NodeInfo{
		node("a", 1, "10.181.0.1", ""),
		node("b", 2, "10.181.0.2", ""),
	}
	inventory, _, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}

	p, ok := sink.received["b"].PeerByNodeID("a")
	if !ok {
		t.Fatal("node b has no peer entry for node a")
	}
	if p.Endpoint != "203.0.113.9:51820" {
		t.Errorf("peer endpoint = %q, want the observed one, not the node's own view", p.Endpoint)
	}
	for _, n := range inventory {
		if n.ID == "a" && n.Endpoint != "203.0.113.9:51820" {
			t.Errorf("inventory endpoint = %q, want the observed one", n.Endpoint)
		}
	}
}

func TestCoordinator_SelfReportedEndpointUsedWhenNoneObserved(t *testing.T) {
	sink := newRecordingSink()
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(1), Endpoint: "203.0.113.9:51820"}
	sink.identities["b"] = NodeIdentity{PublicKey: testKey(2)}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	inventory := []NodeInfo{node("a", 1, "10.181.0.1", ""), node("b", 2, "10.181.0.2", "")}

	inventory, result, err := c.Distribute(context.Background(), inventory)
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != "a" {
		t.Fatalf("Updated = %v, want [a]", result.Updated)
	}
	for _, n := range inventory {
		if n.ID == "a" && n.Endpoint != "203.0.113.9:51820" {
			t.Errorf("endpoint = %q, want the node's self-reported one", n.Endpoint)
		}
	}
}

func TestCoordinator_IgnoresAnUnusableSelfReportedEndpoint(t *testing.T) {
	sink := newRecordingSink()
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(1), Endpoint: "not an endpoint"}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	inventory, _, err := c.Distribute(context.Background(), []NodeInfo{node("a", 1, "10.181.0.1", "")})
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}
	if inventory[0].Endpoint != "" {
		t.Errorf("endpoint = %q, want the malformed value to be ignored", inventory[0].Endpoint)
	}
}

func TestCoordinator_ForgetsAnEndpointWhenANodeDisconnects(t *testing.T) {
	// A stale endpoint for a node on a dynamic address is worse than
	// none: WireGuard keeps sending handshakes to whoever holds it now.
	c := NewCoordinator(newRecordingSink(), PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	if err := c.SetObservedEndpoint("a", "203.0.113.9:51820"); err != nil {
		t.Fatalf("SetObservedEndpoint: %v", err)
	}
	if err := c.SetObservedEndpoint("a", ""); err != nil {
		t.Fatalf("SetObservedEndpoint(clear): %v", err)
	}
	if _, still := c.observedEndpoints["a"]; still {
		t.Error("the observed endpoint survived a disconnect")
	}
}

func TestCoordinator_SetObservedEndpointFailures(t *testing.T) {
	c := NewCoordinator(newRecordingSink(), PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))

	if err := c.SetObservedEndpoint("", "203.0.113.9:51820"); !errors.Is(err, ErrUnknownNode) {
		t.Errorf("empty node ID: error = %v, want it to wrap %v", err, ErrUnknownNode)
	}
	if err := c.SetObservedEndpoint("a", "203.0.113.9"); err == nil {
		t.Error("an endpoint with no port was accepted")
	}
}

func TestCoordinator_ReplacesAReportedKeyChange(t *testing.T) {
	// A rebuilt machine generates a fresh key. The old one is genuinely
	// dead, so refusing the new one would strand the node permanently.
	sink := newRecordingSink()
	sink.identities["a"] = NodeIdentity{PublicKey: testKey(50)}

	c := NewCoordinator(sink, PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	inventory, result, err := c.Distribute(context.Background(), []NodeInfo{node("a", 1, "10.181.0.1", "")})
	if err != nil {
		t.Fatalf("Distribute: %v", err)
	}
	if inventory[0].PublicKey != testKey(50) {
		t.Error("the newly reported key was not adopted")
	}
	if len(result.Updated) != 1 {
		t.Errorf("Updated = %v, want the key change reported so the caller persists it", result.Updated)
	}
}

func TestCoordinator_InventoryProblemsFailThePass(t *testing.T) {
	tests := []struct {
		name    string
		nodes   []NodeInfo
		opts    PlanOptions
		wantErr error
	}{
		{
			name: "duplicate public key",
			nodes: []NodeInfo{
				node("a", 1, "10.181.0.1", ""),
				node("b", 1, "10.181.0.2", ""),
			},
			opts:    PlanOptions{MeshCIDR: testCIDR()},
			wantErr: ErrDuplicatePublicKey,
		},
		{
			name: "address outside the mesh",
			nodes: []NodeInfo{
				node("a", 1, "10.181.0.1", ""),
				node("b", 2, "172.16.0.1", ""),
			},
			opts:    PlanOptions{MeshCIDR: testCIDR()},
			wantErr: ErrAddressOutsideMesh,
		},
		{
			name:    "exhausted cidr",
			nodes:   []NodeInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
			opts:    PlanOptions{MeshCIDR: netip.MustParsePrefix("192.168.42.0/30")},
			wantErr: ErrNoAddressAvailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := newRecordingSink()
			c := NewCoordinator(sink, tc.opts, WithCoordinatorLogger(quietLogger()))
			_, _, err := c.Distribute(context.Background(), tc.nodes)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want it to wrap %v", err, tc.wantErr)
			}
			// Nothing must have been distributed: a wrong plan sent to
			// half the fleet is worse than no plan at all.
			if len(sink.received) != 0 {
				t.Errorf("configs were distributed despite a bad inventory: %v", sink.order)
			}
		})
	}
}

func TestCoordinator_DefaultsTheMeshCIDR(t *testing.T) {
	sink := newRecordingSink()
	c := NewCoordinator(sink, PlanOptions{}, WithCoordinatorLogger(quietLogger()))

	inventory, _, err := c.Distribute(context.Background(), []NodeInfo{{ID: "a"}})
	if err != nil {
		t.Fatalf("Distribute with no mesh CIDR: %v", err)
	}
	if !DefaultMeshCIDR.Contains(inventory[0].Address) {
		t.Errorf("address %s is not in the default mesh CIDR %s", inventory[0].Address, DefaultMeshCIDR)
	}
}

func TestCoordinator_RespectsContextCancellation(t *testing.T) {
	c := NewCoordinator(newRecordingSink(), PlanOptions{MeshCIDR: testCIDR()}, WithCoordinatorLogger(quietLogger()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Distribute(ctx, []NodeInfo{node("a", 1, "10.181.0.1", "")})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Distribute = %v, want it to wrap context.Canceled", err)
	}
}

func TestLocalSink(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	mesh := NewDisabled("test")
	sink, err := NewLocalSink("self", mesh, priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	cfg := DeviceConfig{NodeID: "self", ListenPort: 51820}
	identity, err := sink.ApplyMesh(context.Background(), "self", cfg)
	if err != nil {
		t.Fatalf("ApplyMesh: %v", err)
	}
	if identity.PublicKey != pub {
		t.Error("the reported public key does not match the private key held locally")
	}
	if identity.ListenPort != 51820 {
		t.Errorf("ListenPort = %d, want 51820", identity.ListenPort)
	}

	// The private key is injected at the last possible moment, on the way
	// into the mesh, and only there.
	applied, err := mesh.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if applied.ListenPort != 51820 {
		t.Errorf("the config never reached the mesh: %+v", applied)
	}
	if sink.PublicKey() != pub {
		t.Error("PublicKey() disagrees with the derived public key")
	}
}

func TestLocalSink_RefusesAnotherNodesConfig(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	sink, err := NewLocalSink("self", NewDisabled("test"), priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}

	_, err = sink.ApplyMesh(context.Background(), "somebody-else", DeviceConfig{NodeID: "somebody-else"})
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrUnknownNode)
	}
}

func TestNewLocalSink_Failures(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}

	tests := []struct {
		name    string
		nodeID  string
		mesh    Mesh
		key     Key
		wantErr error
	}{
		{name: "no node id", nodeID: "", mesh: NewDisabled("test"), key: priv, wantErr: ErrUnknownNode},
		{name: "no key", nodeID: "self", mesh: NewDisabled("test"), key: Key{}, wantErr: ErrZeroKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewLocalSink(tc.nodeID, tc.mesh, tc.key); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want it to wrap %v", err, tc.wantErr)
			}
		})
	}

	if _, err := NewLocalSink("self", nil, priv); err == nil {
		t.Error("a nil mesh was accepted")
	}
}

func TestLocalSink_PropagatesMeshFailures(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	mesh := NewDisabled("test")
	if err := mesh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sink, err := NewLocalSink("self", mesh, priv)
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	if _, err := sink.ApplyMesh(context.Background(), "self", DeviceConfig{NodeID: "self"}); !errors.Is(err, ErrMeshClosed) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrMeshClosed)
	}
}
