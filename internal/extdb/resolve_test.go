package extdb

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResolveSourceFollowsRecreatedContainer(t *testing.T) {
	rt := &fakeRuntime{states: []docker.ContainerState{
		{ID: "new", Name: "pg", Image: "postgres:16", Running: true, Networks: []docker.ContainerNetwork{
			{Name: "bridge", IPAddress: "172.17.0.4"}, {Name: "app-net-2", IPAddress: "10.9.0.7"},
		}},
	}}
	cases := []struct {
		name         string
		in           Conn
		wantHost     string
		wantNet      string
		wantNoChange bool
	}{
		{"stale network replaced", Conn{Host: "pg", Network: "app-net-1", SourceContainer: "pg"}, "pg", "app-net-2", false},
		{"stale ip replaced by name", Conn{Host: "10.9.0.3", Network: "app-net-1", SourceContainer: "pg"}, "pg", "app-net-2", false},
		{"recorded network still attached", Conn{Host: "pg", Network: "app-net-2", SourceContainer: "pg"}, "pg", "app-net-2", true},
		{"no source container recorded", Conn{Host: "db.example.com", Network: "x"}, "db.example.com", "x", true},
		{"source missing leaves record alone", Conn{Host: "gone", Network: "n", SourceContainer: "gone"}, "gone", "n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSource(t.Context(), rt, tc.in)
			if got.Host != tc.wantHost || got.Network != tc.wantNet {
				t.Fatalf("got host %q network %q, want %q %q", got.Host, got.Network, tc.wantHost, tc.wantNet)
			}
		})
	}
}

func TestHelperJoinsSourceNetworkAndNeverConnectsSource(t *testing.T) {
	rt := &fakeRuntime{states: []docker.ContainerState{
		{ID: "c1", Name: "pg", Image: "pgvector/pgvector:pg16", Running: true, Networks: []docker.ContainerNetwork{{Name: "coolify-net", IPAddress: "10.5.0.2"}}},
	}}
	h := &Helper{Runtime: rt}
	_, remove, err := h.Start(t.Context(), "main", Conn{Engine: EnginePostgres, Host: "10.5.0.9", Port: 5432, Network: "old-net", SourceContainer: "pg"})
	if err != nil {
		t.Fatal(err)
	}
	remove()
	spec := rt.created[0]
	if spec.Network == nil || spec.Network.Name != "coolify-net" {
		t.Fatalf("helper network = %+v, want coolify-net", spec.Network)
	}
	if spec.Env["PGHOST"] != "pg" {
		t.Errorf("helper PGHOST = %q, want the container name", spec.Env["PGHOST"])
	}
	if spec.Image != "pgvector/pgvector:pg16" {
		t.Errorf("helper image = %q, want the source's own image (no pull)", spec.Image)
	}
}

func TestRefreshReportsChange(t *testing.T) {
	rt := &fakeRuntime{states: []docker.ContainerState{
		{Name: "pg", Running: true, Networks: []docker.ContainerNetwork{{Name: "n2", IPAddress: "10.1.1.1"}}},
	}}
	rec := store.ExternalDatabase{Name: "x", Host: "pg", Network: "n1", SourceContainer: "pg"}
	next, changed := Refresh(t.Context(), rt, rec)
	if !changed || next.Network != "n2" {
		t.Fatalf("next = %+v changed = %v", next, changed)
	}
	if _, changed := Refresh(t.Context(), rt, next); changed {
		t.Error("second refresh should be a no-op")
	}
}
