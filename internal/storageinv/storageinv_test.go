package storageinv

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestIsProtectedVolume(t *testing.T) {
	cases := []struct {
		name string
		v    Volume
		want bool
	}{
		{"db prefix", Volume{Name: "db-foo-data"}, true},
		{"database owned without prefix", Volume{Name: "pgdata", Database: "foo"}, true},
		{"flagged database", Volume{Name: "x", IsDatabase: true}, true},
		{"ordinary volume", Volume{Name: "app-web-uploads"}, false},
		{"db substring only", Volume{Name: "mydb-cache"}, false},
	}
	for _, tc := range cases {
		got, reason := IsProtectedVolume(tc.v)
		if got != tc.want {
			t.Errorf("%s: protected=%v want %v", tc.name, got, tc.want)
		}
		if got && reason == "" {
			t.Errorf("%s: protected without a reason", tc.name)
		}
	}
}

func TestBuildVolumesOwnerAndOrphan(t *testing.T) {
	raw := []docker.InventoryVolume{
		{Name: "app-web-data", SizeBytes: 10, InUse: true},
		{Name: "db-my-db-data", SizeBytes: 5},
		{Name: "app-gone-data", SizeBytes: -1},
		{Name: "labelled", SizeBytes: 1, Labels: map[string]string{docker.VolumeOwnerAppLabel: "blog"}},
	}
	o := Owners{
		VolumeApp:  map[string]string{"app-web-data": "web"},
		Databases:  map[string]bool{"my-db": true, "my": true},
		Referenced: map[string]bool{"app-web-data": true},
	}
	by := map[string]Volume{}
	for _, v := range BuildVolumes(raw, o) {
		by[v.Name] = v
	}
	if by["app-web-data"].OwnerApp != "web" || by["app-web-data"].Orphaned {
		t.Errorf("web volume: %+v", by["app-web-data"])
	}
	if d := by["db-my-db-data"]; d.Database != "my-db" || !d.IsDatabase || !d.Protected || d.Orphaned {
		t.Errorf("db volume: %+v", d)
	}
	if g := by["app-gone-data"]; !g.Orphaned || g.SizeBytes != nil || g.Protected {
		t.Errorf("gone volume: %+v", g)
	}
	if by["labelled"].OwnerApp != "blog" {
		t.Errorf("label owner: %+v", by["labelled"])
	}
}

func TestBuildContainersOwner(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	raw := []docker.InventoryContainer{
		{ID: "a", Name: "web-abc", SizeBytes: 3, ExitedAt: at, Labels: map[string]string{containerAppLabel: "web"}},
		{ID: "b", Name: "api-123456", SizeBytes: 0},
		{ID: "c", Name: "stray", SizeBytes: -1},
	}
	o := Owners{ContainerApp: func(n string) string {
		if n == "api-123456" {
			return "api"
		}
		return ""
	}}
	got := BuildContainers(raw, o)
	if len(got) != 3 || got[0].OwnerApp != "api" || got[2].OwnerApp != "web" || got[2].ExitedAt == nil {
		t.Fatalf("containers: %+v", got)
	}
	if got[1].OwnerApp != "" || got[1].SizeBytes != nil {
		t.Errorf("stray: %+v", got[1])
	}
}
