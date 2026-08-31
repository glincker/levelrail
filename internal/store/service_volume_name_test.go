package store

import (
	"context"
	"errors"
	"testing"
)

func TestServiceVolumeDockerName(t *testing.T) {
	svc := DesiredService{
		Name: "web",
		Volumes: []ServiceVolume{
			{Name: "app-web-data", ContainerPath: "/var/lib/data"},
			{Name: "app-web-cache-dir", ContainerPath: "/var/cache"},
		},
	}

	cases := []struct {
		name       string
		volumeName string
		wantDocker string
		wantOK     bool
	}{
		{name: "simple logical name", volumeName: "data", wantDocker: "app-web-data", wantOK: true},
		{name: "logical name containing a dash", volumeName: "cache-dir", wantDocker: "app-web-cache-dir", wantOK: true},
		{name: "unknown logical name", volumeName: "missing", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ServiceVolumeDockerName(svc, tc.volumeName)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.wantDocker {
				t.Errorf("docker volume name = %q, want %q", got, tc.wantDocker)
			}
		})
	}
}

func TestResolveServiceVolumeDockerName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{
		Name:    "web",
		Image:   "levelrail/thesvg:abc1234",
		Volumes: []ServiceVolume{{Name: "app-web-data", ContainerPath: "/var/lib/data"}},
	}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.ResolveServiceVolumeDockerName(ctx, "web", "data")
	if err != nil {
		t.Fatalf("ResolveServiceVolumeDockerName() error = %v", err)
	}
	if got != "app-web-data" {
		t.Errorf("got %q, want %q", got, "app-web-data")
	}

	if _, err := db.ResolveServiceVolumeDockerName(ctx, "web", "missing"); !errors.Is(err, ErrServiceVolumeNotFound) {
		t.Errorf("ResolveServiceVolumeDockerName() for an unknown volume error = %v, want ErrServiceVolumeNotFound", err)
	}
	if _, err := db.ResolveServiceVolumeDockerName(ctx, "nope", "data"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("ResolveServiceVolumeDockerName() for an unknown service error = %v, want ErrServiceNotFound", err)
	}
}
