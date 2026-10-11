package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDBEndpoints map[string]meshpath.Path

func (f fakeDBEndpoints) Endpoint(_ context.Context, nodeID string) (meshpath.Path, error) {
	return f[nodeID], nil
}

type fakeDBPorts struct{ calls int }

func (f *fakeDBPorts) EnsureDatabaseMeshPort(context.Context, string, int, int) (int, error) {
	f.calls++
	return 21007, nil
}

func TestSameNode(t *testing.T) {
	tests := []struct {
		a, b, local string
		want        bool
	}{
		{"", "", "", true},
		{"", "cp", "cp", true},
		{"cp", "", "cp", true},
		{"n1", "n1", "cp", true},
		{"n1", "n2", "cp", false},
		{"", "n2", "", false},
	}
	for _, tt := range tests {
		if got := SameNode(tt.a, tt.b, tt.local); got != tt.want {
			t.Errorf("SameNode(%q, %q, %q) = %v, want %v", tt.a, tt.b, tt.local, got, tt.want)
		}
	}
}

func TestCrossNodeDatabase(t *testing.T) {
	db := &store.DesiredDatabase{Name: "Main", NodeID: "n2"}
	tests := []struct {
		name      string
		appNode   string
		cfg       *MeshDatabaseConfig
		wantCross bool
		wantErr   string
		wantHost  string
	}{
		{name: "same node uses Docker networking", appNode: "n2", cfg: &MeshDatabaseConfig{Zone: "lr"}},
		{name: "cross node without mesh fails loudly", appNode: "n1", wantErr: "need mesh networking"},
		{name: "cross node over the mesh", appNode: "n1", wantCross: true, wantHost: "main.lr",
			cfg: &MeshDatabaseConfig{Zone: "lr", Endpoints: fakeDBEndpoints{"n2": {Usable: true, Address: "10.181.0.2"}}}},
		{name: "cross node with the mesh path down", appNode: "n1", wantErr: "no usable mesh path",
			cfg: &MeshDatabaseConfig{Zone: "lr", Endpoints: fakeDBEndpoints{"n2": {Reason: "no handshake"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Controller{}
			ports := &fakeDBPorts{}
			if tt.cfg != nil {
				tt.cfg.Ports = ports
				c.meshDB = tt.cfg
			}
			ep, cross, err := c.crossNodeDatabase(context.Background(), tt.appNode, db)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || cross != tt.wantCross {
				t.Fatalf("cross = %v, err = %v; want cross %v", cross, err, tt.wantCross)
			}
			if cross && (ep.host != tt.wantHost || ep.port != 21007 || ep.ip != "10.181.0.2") {
				t.Fatalf("endpoint = %+v", ep)
			}
			if !cross && ports.calls != 0 {
				t.Fatal("allocated a mesh port for a same-node database")
			}
		})
	}
}
