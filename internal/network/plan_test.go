package network

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func testCIDR() netip.Prefix { return netip.MustParsePrefix("10.181.0.0/16") }

func node(id string, seed byte, addr string, endpoint string) NodeInfo {
	n := NodeInfo{ID: id, Name: id, PublicKey: testKey(seed), Endpoint: endpoint}
	if addr != "" {
		n.Address = netip.MustParseAddr(addr)
	}
	return n
}

// TestPlan_FullMeshNotHubAndSpoke is the assertion ADR 006's central
// decision reduces to: every node peers with every other node directly,
// and nobody peers with itself. A hub-and-spoke implementation would pass
// a "peers exist" test and fail this one.
func TestPlan_FullMeshNotHubAndSpoke(t *testing.T) {
	nodes := []NodeInfo{
		node("control", 1, "10.181.0.1", "203.0.113.1:51820"),
		node("worker-a", 2, "10.181.0.2", "203.0.113.2:51820"),
		node("worker-b", 3, "10.181.0.3", "203.0.113.3:51820"),
	}

	plans, err := PlanAll(nodes, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("PlanAll: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("got %d plans, want 3", len(plans))
	}

	for _, n := range nodes {
		cfg := plans[n.ID]
		if len(cfg.Peers) != 2 {
			t.Errorf("node %q has %d peers, want 2 (every other node)", n.ID, len(cfg.Peers))
		}
		if _, self := cfg.PeerByNodeID(n.ID); self {
			t.Errorf("node %q peers with itself", n.ID)
		}
		for _, other := range nodes {
			if other.ID == n.ID {
				continue
			}
			p, ok := cfg.PeerByNodeID(other.ID)
			if !ok {
				t.Fatalf("node %q has no peer entry for %q: this is hub-and-spoke, not a full mesh", n.ID, other.ID)
			}
			if p.PublicKey != other.PublicKey {
				t.Errorf("node %q peer %q has the wrong public key", n.ID, other.ID)
			}
			if p.Endpoint != other.Endpoint {
				t.Errorf("node %q peer %q endpoint = %q, want %q", n.ID, other.ID, p.Endpoint, other.Endpoint)
			}
			want := netip.PrefixFrom(other.Address, 32)
			if len(p.AllowedIPs) != 1 || p.AllowedIPs[0] != want {
				t.Errorf("node %q peer %q AllowedIPs = %v, want exactly [%s]", n.ID, other.ID, p.AllowedIPs, want)
			}
		}
	}
}

func TestPlan_SelfConfig(t *testing.T) {
	nodes := []NodeInfo{
		node("a", 1, "10.181.0.1", ""),
		node("b", 2, "10.181.0.2", ""),
	}

	cfg, err := Plan("a", nodes, PlanOptions{MeshCIDR: testCIDR(), ListenPort: 5555, Keepalive: 10 * time.Second})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if cfg.NodeID != "a" {
		t.Errorf("NodeID = %q, want %q", cfg.NodeID, "a")
	}
	if got, want := cfg.Address.String(), "10.181.0.1/16"; got != want {
		t.Errorf("Address = %q, want %q (the mesh prefix length, not /32, so the host routes the whole mesh)", got, want)
	}
	if cfg.ListenPort != 5555 {
		t.Errorf("ListenPort = %d, want 5555", cfg.ListenPort)
	}
	if !cfg.PrivateKey.IsZero() {
		t.Error("Plan produced a private key: the control plane must never hold one")
	}
	if cfg.Peers[0].PersistentKeepalive != 10*time.Second {
		t.Errorf("PersistentKeepalive = %v, want 10s", cfg.Peers[0].PersistentKeepalive)
	}
}

func TestPlanOptions_Defaults(t *testing.T) {
	tests := []struct {
		name          string
		opts          PlanOptions
		wantPort      int
		wantKeepalive time.Duration
	}{
		{
			name:          "zero means the defaults",
			opts:          PlanOptions{MeshCIDR: testCIDR()},
			wantPort:      DefaultListenPort,
			wantKeepalive: DefaultKeepalive,
		},
		{
			name:          "negative keepalive means explicitly off",
			opts:          PlanOptions{MeshCIDR: testCIDR(), Keepalive: -1},
			wantPort:      DefaultListenPort,
			wantKeepalive: 0,
		},
		{
			name:          "explicit values win",
			opts:          PlanOptions{MeshCIDR: testCIDR(), ListenPort: 1234, Keepalive: time.Minute},
			wantPort:      1234,
			wantKeepalive: time.Minute,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := []NodeInfo{node("a", 1, "10.181.0.1", ""), node("b", 2, "10.181.0.2", "")}
			cfg, err := Plan("a", nodes, tc.opts)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if cfg.ListenPort != tc.wantPort {
				t.Errorf("ListenPort = %d, want %d", cfg.ListenPort, tc.wantPort)
			}
			if cfg.Peers[0].PersistentKeepalive != tc.wantKeepalive {
				t.Errorf("PersistentKeepalive = %v, want %v", cfg.Peers[0].PersistentKeepalive, tc.wantKeepalive)
			}
		})
	}
}

// TestPlan_NotReadyNodesAreSkippedNotFatal covers the convergence
// property: a node that enrolled but has not reported a key yet must not
// stop the rest of the fleet from meshing.
func TestPlan_NotReadyNodesAreSkippedNotFatal(t *testing.T) {
	nodes := []NodeInfo{
		node("ready-a", 1, "10.181.0.1", ""),
		node("ready-b", 2, "10.181.0.2", ""),
		{ID: "no-key", Name: "no-key", Address: netip.MustParseAddr("10.181.0.3")},
		{ID: "no-address", Name: "no-address", PublicKey: testKey(4)},
	}

	cfg, err := Plan("ready-a", nodes, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(cfg.Peers) != 1 {
		t.Fatalf("got %d peers, want only the one ready node: %+v", len(cfg.Peers), cfg.Peers)
	}
	if cfg.Peers[0].NodeID != "ready-b" {
		t.Errorf("peer = %q, want %q", cfg.Peers[0].NodeID, "ready-b")
	}

	// A node with no key of its own still gets a valid plan naming its
	// peers: applying that plan is what produces the key it reports back.
	bootstrap, err := Plan("no-key", nodes, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("Plan for a keyless node: %v", err)
	}
	if len(bootstrap.Peers) != 2 {
		t.Errorf("keyless node got %d peers, want 2", len(bootstrap.Peers))
	}
}

func TestPlan_Deterministic(t *testing.T) {
	nodes := []NodeInfo{
		node("c", 3, "10.181.0.3", ""),
		node("a", 1, "10.181.0.1", ""),
		node("b", 2, "10.181.0.2", ""),
	}
	reordered := []NodeInfo{nodes[1], nodes[2], nodes[0]}

	first, err := Plan("a", nodes, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	second, err := Plan("a", reordered, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(first.Peers) != len(second.Peers) {
		t.Fatalf("peer counts differ: %d vs %d", len(first.Peers), len(second.Peers))
	}
	for i := range first.Peers {
		if first.Peers[i].NodeID != second.Peers[i].NodeID {
			t.Fatalf("peer order depends on inventory order: %q vs %q at index %d",
				first.Peers[i].NodeID, second.Peers[i].NodeID, i)
		}
	}
}

func TestPlan_Failures(t *testing.T) {
	tests := []struct {
		name    string
		selfID  string
		nodes   []NodeInfo
		cidr    netip.Prefix
		wantErr error
	}{
		{
			name:    "unknown self",
			selfID:  "ghost",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    testCIDR(),
			wantErr: ErrUnknownNode,
		},
		{
			name:    "empty self",
			selfID:  "",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    testCIDR(),
			wantErr: ErrUnknownNode,
		},
		{
			name:   "two nodes share a public key",
			selfID: "a",
			nodes: []NodeInfo{
				node("a", 1, "10.181.0.1", ""),
				node("b", 1, "10.181.0.2", ""),
			},
			cidr:    testCIDR(),
			wantErr: ErrDuplicatePublicKey,
		},
		{
			name:   "two nodes share an address",
			selfID: "a",
			nodes: []NodeInfo{
				node("a", 1, "10.181.0.1", ""),
				node("b", 2, "10.181.0.1", ""),
			},
			cidr:    testCIDR(),
			wantErr: ErrDuplicateAddress,
		},
		{
			name:   "address outside the mesh cidr",
			selfID: "a",
			nodes: []NodeInfo{
				node("a", 1, "10.181.0.1", ""),
				node("b", 2, "192.168.1.5", ""),
			},
			cidr:    testCIDR(),
			wantErr: ErrAddressOutsideMesh,
		},
		{
			name:    "unset mesh cidr",
			selfID:  "a",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    netip.Prefix{},
			wantErr: ErrInvalidMeshCIDR,
		},
		{
			name:    "mesh cidr with host bits set",
			selfID:  "a",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    netip.MustParsePrefix("10.181.0.7/16"),
			wantErr: ErrInvalidMeshCIDR,
		},
		{
			name:    "ipv6 mesh cidr",
			selfID:  "a",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    netip.MustParsePrefix("fd00::/64"),
			wantErr: ErrInvalidMeshCIDR,
		},
		{
			name:    "mesh cidr too small to hold a mesh",
			selfID:  "a",
			nodes:   []NodeInfo{node("a", 1, "10.181.0.1", "")},
			cidr:    netip.MustParsePrefix("10.181.0.0/31"),
			wantErr: ErrInvalidMeshCIDR,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Plan(tc.selfID, tc.nodes, PlanOptions{MeshCIDR: tc.cidr})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Plan error = %v, want it to wrap %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateInventory_DuplicateNodeID(t *testing.T) {
	nodes := []NodeInfo{
		node("a", 1, "10.181.0.1", ""),
		node("a", 2, "10.181.0.2", ""),
	}
	err := ValidateInventory(nodes, testCIDR())
	if err == nil {
		t.Fatal("want an error for a duplicated node ID, got nil")
	}
}

func TestValidateInventory_EmptyNodeID(t *testing.T) {
	if err := ValidateInventory([]NodeInfo{{ID: ""}}, testCIDR()); err == nil {
		t.Fatal("want an error for a node with no ID, got nil")
	}
}

func TestValidateInventory_SingleNodeIsValid(t *testing.T) {
	// The single-node case has no peers at all and must still be a valid
	// inventory: ADR 006's "single-node deployments do not pay any
	// WireGuard cost" only holds if the same code path runs cleanly.
	if err := ValidateInventory([]NodeInfo{node("only", 1, "10.181.0.1", "")}, testCIDR()); err != nil {
		t.Fatalf("single-node inventory rejected: %v", err)
	}
	cfg, err := Plan("only", []NodeInfo{node("only", 1, "10.181.0.1", "")}, PlanOptions{MeshCIDR: testCIDR()})
	if err != nil {
		t.Fatalf("Plan for a single node: %v", err)
	}
	if len(cfg.Peers) != 0 {
		t.Errorf("single node has %d peers, want 0", len(cfg.Peers))
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty is not an error", input: "", want: ""},
		{name: "ipv4", input: "203.0.113.4:51820", want: "203.0.113.4:51820"},
		{name: "dns name", input: "node.example.com:51820", want: "node.example.com:51820"},
		{name: "bracketed ipv6", input: "[fd00::1]:51820", want: "[fd00::1]:51820"},
		{name: "non canonical ipv6 is normalized", input: "[FD00:0:0::0:1]:51820", want: "[fd00::1]:51820"},
		{name: "malformed ipv4 is not treated as a hostname", input: "010.000.113.4:51820", wantErr: true},
		{name: "out of range ipv4 octet", input: "10.0.0.256:51820", wantErr: true},
		{name: "no port", input: "203.0.113.4", wantErr: true},
		{name: "port zero", input: "203.0.113.4:0", wantErr: true},
		{name: "port too large", input: "203.0.113.4:70000", wantErr: true},
		{name: "non numeric port", input: "203.0.113.4:http", wantErr: true},
		{name: "empty host", input: ":51820", wantErr: true},
		{name: "unbracketed ipv6", input: "fd00::1:51820", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEndpoint(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseEndpoint(%q) = %q, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEndpoint(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseEndpoint(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
