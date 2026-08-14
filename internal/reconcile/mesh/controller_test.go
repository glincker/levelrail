package mesh

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discard{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

type fakeStore struct {
	nodes     []store.Node
	services  []store.DesiredService
	databases []store.DesiredDatabase

	nodesErr    error
	servicesErr error
	updateErr   error

	updates []meshUpdate
}

type meshUpdate struct {
	nodeID    string
	publicKey string
	address   string
}

func (f *fakeStore) ListNodes(context.Context) ([]store.Node, error) { return f.nodes, f.nodesErr }

func (f *fakeStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return f.services, f.servicesErr
}

func (f *fakeStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return f.databases, nil
}

func (f *fakeStore) UpdateNodeMesh(_ context.Context, id, publicKey, address string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updates = append(f.updates, meshUpdate{nodeID: id, publicKey: publicKey, address: address})
	return nil
}

// realDistributor wires the controller to a real network.Coordinator over
// a sink that answers the way a node would. Using the real coordinator
// rather than a fake one is deliberate: the interesting behavior of this
// controller is what it does with a real distribution result, and a fake
// distributor would let the two drift apart.
type nodeSink struct {
	keys map[string]network.Key
	fail map[string]error
}

func (s nodeSink) ApplyMesh(_ context.Context, nodeID string, _ network.DeviceConfig) (network.NodeIdentity, error) {
	if err, bad := s.fail[nodeID]; bad {
		return network.NodeIdentity{}, err
	}
	return network.NodeIdentity{PublicKey: s.keys[nodeID], ListenPort: network.DefaultListenPort}, nil
}

func key(seed byte) network.Key {
	var k network.Key
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}

func newController(t *testing.T, st *fakeStore, sink network.ConfigSink) (*Controller, *network.Resolver) {
	t.Helper()
	resolver := network.NewResolver(network.Zone("acme"))
	coord := network.NewCoordinator(sink,
		network.PlanOptions{MeshCIDR: netip.MustParsePrefix("10.181.0.0/16")},
		network.WithCoordinatorLogger(quietLogger()))
	return New("control", st, coord, resolver, WithLogger(quietLogger())), resolver
}

func conditionByType(t *testing.T, res reconcile.Result, kind string) reconcile.Condition {
	t.Helper()
	for _, c := range res.Conditions {
		if c.Type == kind {
			return c
		}
	}
	t.Fatalf("no %q condition in %+v", kind, res.Conditions)
	return reconcile.Condition{}
}

// TestReconcile_ConvergesAFleetFromNothing walks the whole subsystem from
// a store where no node has ever meshed: addresses get allocated, keys
// come back from the nodes, both get persisted, and every service name
// resolves.
func TestReconcile_ConvergesAFleetFromNothing(t *testing.T) {
	st := &fakeStore{
		nodes: []store.Node{
			{ID: "control", Name: "control"},
			{ID: "worker", Name: "worker", Address: "203.0.113.7:51820"},
		},
		services:  []store.DesiredService{{Name: "web", NodeID: "worker"}},
		databases: []store.DesiredDatabase{{Name: "postgres"}},
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1), "worker": key(2)}}

	c, resolver := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := conditionByType(t, res, "MeshPlanned"); got.Status != reconcile.ConditionTrue {
		t.Errorf("MeshPlanned = %s (%s)", got.Status, got.Message)
	}
	if got := conditionByType(t, res, "MeshDistributed"); got.Status != reconcile.ConditionTrue {
		t.Errorf("MeshDistributed = %s (%s)", got.Status, got.Message)
	}
	if got := conditionByType(t, res, "DNSResolvable"); got.Status != reconcile.ConditionTrue {
		t.Errorf("DNSResolvable = %s (%s)", got.Status, got.Message)
	}

	// Both nodes' keys and addresses were persisted, so the next pass
	// plans from them rather than starting over.
	if len(st.updates) != 2 {
		t.Fatalf("got %d mesh state writes, want 2: %+v", len(st.updates), st.updates)
	}
	for _, u := range st.updates {
		if u.publicKey == "" || u.address == "" {
			t.Errorf("node %q persisted with an empty key or address: %+v", u.nodeID, u)
		}
	}

	// The database placed on the control plane's own node (node_id "")
	// resolves, which is the exact case 0009's empty-string convention
	// makes non-obvious.
	zone := network.Zone("acme")
	if _, ok := resolver.Lookup("postgres." + zone); !ok {
		t.Error("a service on the control plane's own node does not resolve")
	}
	if addr, ok := resolver.Lookup("web." + zone); !ok {
		t.Error("a service on the worker does not resolve")
	} else if addr.String() == "" {
		t.Error("web resolved to an empty address")
	}
}

// TestReconcile_MovedServiceResolvesToItsNewNode is the exit criterion at
// the controller level: the store says the service moved, and the very
// next reconcile pass makes the name point at the new machine.
func TestReconcile_MovedServiceResolvesToItsNewNode(t *testing.T) {
	st := &fakeStore{
		nodes: []store.Node{
			{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"},
			{ID: "worker", Name: "worker", MeshPublicKey: key(2).String(), MeshAddress: "10.181.0.2"},
		},
		databases: []store.DesiredDatabase{{Name: "postgres"}}, // node_id "" == control
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1), "worker": key(2)}}

	c, resolver := newController(t, st, sink)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	zone := network.Zone("acme")
	before, ok := resolver.Lookup("postgres." + zone)
	if !ok || before.String() != "10.181.0.1" {
		t.Fatalf("before the move: postgres resolves to %v (ok=%v), want 10.181.0.1", before, ok)
	}

	// The move, exactly as store.UpdateDatabaseNode would record it.
	st.databases[0].NodeID = "worker"

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	after, ok := resolver.Lookup("postgres." + zone)
	if !ok {
		t.Fatal("after the move: postgres stopped resolving, so every connection string holding it is broken")
	}
	if after.String() != "10.181.0.2" {
		t.Errorf("after the move: postgres resolves to %s, want the worker's 10.181.0.2", after)
	}
}

func TestReconcile_UnreachableNodeIsReportedNotFatal(t *testing.T) {
	st := &fakeStore{
		nodes: []store.Node{
			{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"},
			{ID: "worker", Name: "worker", MeshPublicKey: key(2).String(), MeshAddress: "10.181.0.2"},
		},
		databases: []store.DesiredDatabase{{Name: "postgres"}},
	}
	sink := nodeSink{
		keys: map[string]network.Key{"control": key(1)},
		fail: map[string]error{"worker": errors.New("node not registered in this transport registry")},
	}

	c, resolver := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile returned an error for one unreachable node: %v", err)
	}

	if got := conditionByType(t, res, "MeshDistributed"); got.Status != reconcile.ConditionFalse || got.Reason != "NodeUnreachable" {
		t.Errorf("MeshDistributed = %s/%s, want False/NodeUnreachable", got.Status, got.Reason)
	}
	// The rest of the fleet still converged, and DNS still resolves: an
	// unreachable node must not take the zone down with it.
	if got := conditionByType(t, res, "DNSResolvable"); got.Status != reconcile.ConditionTrue {
		t.Errorf("DNSResolvable = %s (%s)", got.Status, got.Message)
	}
	if _, ok := resolver.Lookup("postgres." + network.Zone("acme")); !ok {
		t.Error("postgres stopped resolving because a different node was unreachable")
	}
}

func TestReconcile_UnresolvablePlacementIsSurfaced(t *testing.T) {
	st := &fakeStore{
		nodes:    []store.Node{{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"}},
		services: []store.DesiredService{{Name: "web", NodeID: "a-node-that-does-not-exist"}},
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1)}}

	c, _ := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := conditionByType(t, res, "DNSResolvable")
	if got.Status != reconcile.ConditionFalse || got.Reason != "PlacementUnresolvable" {
		t.Errorf("DNSResolvable = %s/%s, want False/PlacementUnresolvable", got.Status, got.Reason)
	}
}

func TestReconcile_DuplicateServiceNamesKeepTheOldZone(t *testing.T) {
	// Stale-but-working beats correct-and-empty: replacing the zone with
	// nothing would make every internal name answer NXDOMAIN.
	st := &fakeStore{
		nodes:    []store.Node{{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"}},
		services: []store.DesiredService{{Name: "web"}},
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1)}}

	c, resolver := newController(t, st, sink)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// A database that collides with an existing service name.
	st.databases = []store.DesiredDatabase{{Name: "web"}}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	got := conditionByType(t, res, "DNSResolvable")
	if got.Status != reconcile.ConditionFalse || got.Reason != "RecordsInvalid" {
		t.Errorf("DNSResolvable = %s/%s, want False/RecordsInvalid", got.Status, got.Reason)
	}
	if _, ok := resolver.Lookup("web." + network.Zone("acme")); !ok {
		t.Error("the previous, working zone was replaced with an empty one")
	}
}

func TestReconcile_NodeWithUnusableMeshStateIsExcludedNotFatal(t *testing.T) {
	st := &fakeStore{
		nodes: []store.Node{
			{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"},
			{ID: "corrupt", Name: "corrupt", MeshPublicKey: "this is not a key"},
			{ID: "bad-address", Name: "bad-address", MeshAddress: "not an address"},
		},
		databases: []store.DesiredDatabase{{Name: "postgres"}},
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1)}}

	c, resolver := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := conditionByType(t, res, "MeshPlanned"); got.Status != reconcile.ConditionTrue {
		t.Errorf("MeshPlanned = %s (%s): one corrupt row must not stop the fleet", got.Status, got.Message)
	}
	if _, ok := resolver.Lookup("postgres." + network.Zone("acme")); !ok {
		t.Error("a healthy node's services stopped resolving because another node's row was corrupt")
	}
}

func TestReconcile_StoreFailures(t *testing.T) {
	tests := []struct {
		name      string
		store     *fakeStore
		wantKind  string
		wantState reconcile.ConditionStatus
		wantErr   bool
	}{
		{
			name:      "cannot list nodes",
			store:     &fakeStore{nodesErr: errors.New("database is locked")},
			wantKind:  "MeshPlanned",
			wantState: reconcile.ConditionFalse,
			wantErr:   true,
		},
		{
			name: "cannot list services",
			store: &fakeStore{
				nodes:       []store.Node{{ID: "control", Name: "control", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"}},
				servicesErr: errors.New("database is locked"),
			},
			wantKind:  "DNSResolvable",
			wantState: reconcile.ConditionUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := nodeSink{keys: map[string]network.Key{"control": key(1)}}
			c, _ := newController(t, tc.store, sink)

			res, err := c.Reconcile(context.Background())
			if tc.wantErr && err == nil {
				t.Fatal("want an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Conditions are returned even alongside an error, per
			// reconcile.Controller's contract.
			got := conditionByType(t, res, tc.wantKind)
			if got.Status != tc.wantState {
				t.Errorf("%s = %s, want %s", tc.wantKind, got.Status, tc.wantState)
			}
			if got.Message == "" {
				t.Errorf("%s has no message", tc.wantKind)
			}
		})
	}
}

func TestReconcile_PersistFailureIsReportedNotFatal(t *testing.T) {
	st := &fakeStore{
		nodes:     []store.Node{{ID: "control", Name: "control"}},
		updateErr: errors.New("database is locked"),
	}
	sink := nodeSink{keys: map[string]network.Key{"control": key(1)}}

	c, _ := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := conditionByType(t, res, "MeshDistributed")
	if got.Status != reconcile.ConditionFalse || got.Reason != "PersistFailed" {
		t.Errorf("MeshDistributed = %s/%s, want False/PersistFailed", got.Status, got.Reason)
	}
	if !strings.Contains(got.Message, "every node was configured") {
		t.Errorf("message = %q, want it to say the mesh itself is fine", got.Message)
	}
}

func TestReconcile_InventoryProblemFailsThePass(t *testing.T) {
	// Two nodes sharing a public key would silently misroute: WireGuard
	// identifies peers only by key. Nothing may be distributed.
	st := &fakeStore{nodes: []store.Node{
		{ID: "a", Name: "a", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"},
		{ID: "b", Name: "b", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.2"},
	}}
	sink := nodeSink{keys: map[string]network.Key{}}

	c, _ := newController(t, st, sink)
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("want an error for a duplicate public key, got nil")
	}
	got := conditionByType(t, res, "MeshPlanned")
	if got.Status != reconcile.ConditionFalse || got.Reason != "InventoryInvalid" {
		t.Errorf("MeshPlanned = %s/%s, want False/InventoryInvalid", got.Status, got.Reason)
	}
	if len(st.updates) != 0 {
		t.Errorf("mesh state was persisted despite an invalid inventory: %+v", st.updates)
	}
}

func TestController_Name(t *testing.T) {
	c, _ := newController(t, &fakeStore{}, nodeSink{})
	if c.Name() != "mesh" {
		t.Errorf("Name() = %q, want %q", c.Name(), "mesh")
	}
}

func TestToInventory_UnusableEndpointDoesNotDisqualifyANode(t *testing.T) {
	// WireGuard learns a peer's endpoint from its first inbound
	// handshake, and ADR 003 guarantees the node always initiates, so a
	// missing endpoint costs one handshake round rather than the node's
	// whole presence on the mesh.
	c, _ := newController(t, &fakeStore{}, nodeSink{})
	inventory, invalid := c.toInventory([]store.Node{
		{ID: "n1", Name: "n1", Address: "no port here", MeshPublicKey: key(1).String(), MeshAddress: "10.181.0.1"},
	})
	if len(invalid) != 0 {
		t.Fatalf("node excluded for a bad endpoint: %+v", invalid)
	}
	if len(inventory) != 1 {
		t.Fatalf("got %d nodes, want 1", len(inventory))
	}
	if inventory[0].Endpoint != "" {
		t.Errorf("Endpoint = %q, want it cleared", inventory[0].Endpoint)
	}
	if !inventory[0].Ready() {
		t.Error("the node is not Ready despite having a key and an address")
	}
}
