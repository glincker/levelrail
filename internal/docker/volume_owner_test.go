package docker

import (
	"context"
	"io"
	"testing"
)

func TestIsRootUser(t *testing.T) {
	tests := []struct {
		user string
		want bool
	}{
		{"", true}, {"0", true}, {"root", true}, {"0:0", true}, {"root:wheel", true},
		{"101", false}, {"node", false}, {"1000:1000", false},
	}
	for _, tt := range tests {
		if got := isRootUser(tt.user); got != tt.want {
			t.Errorf("isRootUser(%q) = %v, want %v", tt.user, got, tt.want)
		}
	}
}

func TestLookupPasswdAndGroup(t *testing.T) {
	passwd := []byte("root:x:0:0:root:/root:/bin/sh\nnginx:x:101:102:nginx:/var/cache/nginx:/sbin/nologin\n")
	groups := []byte("root:x:0:\nstaff:x:50:\n")

	if uid, gid, ok := lookupPasswd(passwd, "nginx"); !ok || uid != 101 || gid != 102 {
		t.Errorf("lookupPasswd(nginx) = %d,%d,%v, want 101,102,true", uid, gid, ok)
	}
	if _, _, ok := lookupPasswd(passwd, "ghost"); ok {
		t.Error("lookupPasswd(ghost) found a user that is not there")
	}
	if gid, ok := lookupGroup(groups, "staff"); !ok || gid != 50 {
		t.Errorf("lookupGroup(staff) = %d,%v, want 50,true", gid, ok)
	}
}

// A new volume at a path the image lacks must be writable by the image's
// non-root user; a volume that already holds data must not be touched.
func TestClient_Create_Live_NewVolumeWritableByNonRootImage(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	name := "levelrail-test-volowner"
	vol := "levelrail-test-volowner-data"
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(ctx, t, c, name) })
	_ = c.cli.VolumeRemove(ctx, vol, true)
	t.Cleanup(func() { _ = c.cli.VolumeRemove(ctx, vol, true) })

	if err := c.EnsureVolume(ctx, vol); err != nil {
		t.Fatalf("EnsureVolume: %v", err)
	}
	id, err := c.Create(ctx, ContainerSpec{
		Name:    name,
		Image:   "nginxinc/nginx-unprivileged:1.27-alpine",
		Command: []string{"sleep", "60"},
		Volumes: []VolumeMount{{Name: vol, ContainerPath: "/srv/store"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}

	rc, err := c.Exec(ctx, id, []string{"sh", "-c", "touch /srv/store/probe && echo writable"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("exec touch failed, volume is not writable by the non-root user: %v (%s)", err, out)
	}
}
