package meshpath

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMesh struct {
	st  network.Status
	err error
}

func (f fakeMesh) Status(context.Context) (network.Status, error) { return f.st, f.err }

type fakeNodes map[string]store.Node

func (f fakeNodes) GetNode(_ context.Context, id string) (*store.Node, error) {
	n, ok := f[id]
	if !ok {
		return nil, store.ErrNodeNotFound
	}
	return &n, nil
}

func newKey(t *testing.T) network.Key {
	t.Helper()
	k, err := network.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestValidateBindIP(t *testing.T) {
	tests := []struct {
		ip      string
		wantErr bool
	}{
		{"10.181.0.2", false},
		{"100.64.0.7", false},
		{"0.0.0.0", true},
		{"::", true},
		{"127.0.0.1", true},
		{"224.0.0.1", true},
		{"", true},
		{"public", true},
	}
	for _, tt := range tests {
		if err := ValidateBindIP(tt.ip); (err != nil) != tt.wantErr {
			t.Errorf("ValidateBindIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
		}
	}
}

func TestTracker_Path(t *testing.T) {
	now := time.Now()
	peerKey := newKey(t)
	node := store.Node{ID: "node-2", MeshAddress: "10.181.0.2", MeshPublicKey: peerKey.String()}
	status := func(handshake time.Time) network.Status {
		return network.Status{
			Address: netip.MustParsePrefix("10.181.0.1/16"),
			Peers:   []network.PeerStatus{{NodeID: "node-2", PublicKey: peerKey, LastHandshake: handshake}},
		}
	}
	tests := []struct {
		name       string
		nodeID     string
		mesh       StatusReader
		node       store.Node
		wantLocal  bool
		wantUsable bool
		wantReason string
	}{
		{name: "empty id is local", nodeID: "", wantLocal: true},
		{name: "own id is local", nodeID: "node-1", wantLocal: true},
		{name: "mesh off", nodeID: "node-2", mesh: nil, node: node, wantReason: "APP_MESH_ENABLED"},
		{name: "no address yet", nodeID: "node-2", mesh: fakeMesh{st: status(now)}, node: store.Node{ID: "node-2"}, wantReason: "not joined"},
		{name: "device down", nodeID: "node-2", mesh: fakeMesh{}, node: node, wantReason: "not up"},
		{name: "outside mesh", nodeID: "node-2", mesh: fakeMesh{st: status(now)}, node: store.Node{ID: "node-2", MeshAddress: "192.168.1.5", MeshPublicKey: peerKey.String()}, wantReason: "outside"},
		{name: "never handshaked", nodeID: "node-2", mesh: fakeMesh{st: status(time.Time{})}, node: node, wantReason: "handshake"},
		{name: "stale handshake", nodeID: "node-2", mesh: fakeMesh{st: status(now.Add(-time.Hour))}, node: node, wantReason: "handshake"},
		{name: "healthy", nodeID: "node-2", mesh: fakeMesh{st: status(now.Add(-time.Second))}, node: node, wantUsable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewTracker("node-1", tt.mesh, fakeNodes{"node-2": tt.node})
			tr.now = func() time.Time { return now }
			p, err := tr.Path(context.Background(), tt.nodeID)
			if err != nil {
				t.Fatal(err)
			}
			if p.Local != tt.wantLocal || p.Usable != tt.wantUsable {
				t.Fatalf("Path = %+v, want local=%v usable=%v", p, tt.wantLocal, tt.wantUsable)
			}
			if tt.wantUsable && p.Address != "10.181.0.2" {
				t.Errorf("Address = %q", p.Address)
			}
			if !tt.wantUsable && !tt.wantLocal && !strings.Contains(p.Reason, tt.wantReason) {
				t.Errorf("Reason = %q, want %q", p.Reason, tt.wantReason)
			}
		})
	}
}

func TestTracker_Path_Errors(t *testing.T) {
	tr := NewTracker("node-1", fakeMesh{err: errors.New("uapi")}, fakeNodes{"node-2": {ID: "node-2", MeshAddress: "10.181.0.2", MeshPublicKey: "k"}})
	if _, err := tr.Path(context.Background(), "node-2"); err == nil {
		t.Error("status error: want it surfaced")
	}
	if _, err := tr.Path(context.Background(), "missing"); err == nil {
		t.Error("unknown node: want an error")
	}
}

// Databases publish only via their own bind address and must never pick up a
// mesh bind: the database controller may not depend on this package.
func TestDatabaseControllerNeverImportsMeshPath(t *testing.T) {
	dir := filepath.Join("..", "reconcile", "database")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob %s: %v (%d files)", dir, err, len(files))
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // f comes from Glob over a fixed repo directory
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			if strings.HasSuffix(strings.Trim(imp.Path.Value, `"`), "/internal/meshpath") {
				t.Errorf("%s imports meshpath: a database port must never be mesh-bound", f)
			}
		}
	}
}
