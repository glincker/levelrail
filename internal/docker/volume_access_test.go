package docker

import (
	"archive/tar"
	"context"
	"testing"
)

func TestProbeVolumeAccess(t *testing.T) {
	users := "root:x:0:0::/root:/bin/sh\nnextjs:x:1001:1001::/app:/sbin/nologin\n"
	rootOwned := func(t *testing.T) []byte {
		return tarOf(t, &tar.Header{Name: "cache/", Typeflag: tar.TypeDir, Mode: 0o755})
	}
	userOwned := func(t *testing.T) []byte {
		return tarOf(t, &tar.Header{Name: "cache/", Typeflag: tar.TypeDir, Uid: 1001, Gid: 1001, Mode: 0o755})
	}
	tests := []struct {
		name, user   string
		dir          func(*testing.T) []byte
		wantWritable bool
		wantUID      int
	}{
		{"named non-root user, root-owned root", "nextjs", rootOwned, false, 1001},
		{"numeric user owns the root", "1001:1001", userOwned, true, 1001},
		{"empty user is root", "", rootOwned, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeVolumeDaemon{containerUser: tt.user, users: users, dirs: map[string][]byte{"/app/.next/cache": tt.dir(t)}}
			c := f.serve(t)
			got, err := c.ProbeVolumeAccess(context.Background(), "cid", []VolumeMount{
				{Name: "app-web-cache", ContainerPath: "/app/.next/cache"},
				{Name: "ro", ContainerPath: "/etc/ro", ReadOnly: true},
			})
			if err != nil {
				t.Fatalf("ProbeVolumeAccess() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d results, want 1 (read-only mounts skipped): %+v", len(got), got)
			}
			if got[0].Writable != tt.wantWritable || got[0].UID != tt.wantUID || got[0].Path != "/app/.next/cache" {
				t.Errorf("result = %+v, want writable %v uid %d", got[0], tt.wantWritable, tt.wantUID)
			}
		})
	}
}

func TestProbeVolumeAccess_UnknownUserFails(t *testing.T) {
	f := &fakeVolumeDaemon{containerUser: "ghost", users: "root:x:0:0::/root:/bin/sh\n", dirs: map[string][]byte{}}
	c := f.serve(t)
	if _, err := c.ProbeVolumeAccess(context.Background(), "cid", []VolumeMount{{Name: "v", ContainerPath: "/data"}}); err == nil {
		t.Fatal("ProbeVolumeAccess() error = nil, want an unresolved user to fail")
	}
}

var _ VolumeAccessProber = (*Client)(nil)
