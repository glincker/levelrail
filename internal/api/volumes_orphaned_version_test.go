package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type markerVolumes struct {
	files map[string]string
}

func (m markerVolumes) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return nil, nil
}
func (m markerVolumes) RemoveVolume(context.Context, string) error { return nil }
func (m markerVolumes) ReadVolumeFile(_ context.Context, vol, rel string) ([]byte, error) {
	if v, ok := m.files[vol+"/"+rel]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("missing")
}

func TestOrphanVolumeEngineMajor(t *testing.T) {
	rt := &Router{orphanedVolumes: markerVolumes{files: map[string]string{
		"db-old-data/PG_VERSION":           "18\n",
		"db-my-data/mysql_upgrade_info":    "8.4.1",
		"db-plain-data/unrelated":          "x",
		"not-a-database-volume/PG_VERSION": "17",
	}}}
	ctx := context.Background()
	if e, v := rt.orphanVolumeEngineMajor(ctx, "db-old-data"); e != "postgres" || v != "18" {
		t.Fatalf("postgres volume = %q %q", e, v)
	}
	if e, v := rt.orphanVolumeEngineMajor(ctx, "db-my-data"); e != "mysql" || v != "8.4" {
		t.Fatalf("mysql volume = %q %q", e, v)
	}
	if _, v := rt.orphanVolumeEngineMajor(ctx, "db-plain-data"); v != "" {
		t.Fatalf("unreadable volume reported %q", v)
	}
	if _, v := rt.orphanVolumeEngineMajor(ctx, "not-a-database-volume"); v != "" {
		t.Fatalf("non database volume reported %q", v)
	}
}
