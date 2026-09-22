package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// TestClient_ListNamedVolumes_Live is the live proof ListNamedVolumes
// reports exactly the candidates orphan detection needs: a project-named
// volume that's genuinely unattached (SizeBytes and Mounted both
// observable), a project-named volume still mounted into a container
// (Mounted must be true, so a caller never treats it as orphaned even if
// desired state somehow lost track of it), and never an anonymous
// volume at all (isProjectNamedVolume's own gate).
func TestClient_ListNamedVolumes_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("flaky under concurrent Docker daemon load on shared CI runners, covered by the nightly full suite")
	}
	c := liveClient(t)
	ctx := context.Background()

	unattachedName := "app-levelrail-test-orphan-unattached-data"
	if err := c.EnsureVolume(ctx, unattachedName); err != nil {
		t.Fatalf("ensure unattached volume: %v", err)
	}
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), unattachedName, true) })

	mountedName := "app-levelrail-test-orphan-mounted-data"
	if err := c.EnsureVolume(ctx, mountedName); err != nil {
		t.Fatalf("ensure mounted volume: %v", err)
	}
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), mountedName, true) })
	ctrName := "levelrail-test-orphan-mount-container"
	ctrID, err := c.Create(ctx, ContainerSpec{
		Name:  ctrName,
		Image: "nginx:alpine",
		Volumes: []VolumeMount{
			{Name: mountedName, ContainerPath: "/usr/share/nginx/html"},
		},
	})
	if err != nil {
		t.Fatalf("create container mounting volume: %v", err)
	}
	t.Cleanup(func() {
		_ = c.cli.ContainerRemove(context.Background(), ctrID, container.RemoveOptions{Force: true})
	})
	if err := c.Start(ctx, ctrID); err != nil {
		t.Fatalf("start container mounting volume: %v", err)
	}

	anonResp, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{})
	if err != nil {
		t.Fatalf("create anonymous volume: %v", err)
	}
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), anonResp.Name, true) })

	vols, err := c.ListNamedVolumes(ctx)
	if err != nil {
		t.Fatalf("ListNamedVolumes: %v", err)
	}
	byName := make(map[string]NamedVolume, len(vols))
	for _, v := range vols {
		byName[v.Name] = v
	}

	unattached, ok := byName[unattachedName]
	if !ok {
		t.Fatalf("ListNamedVolumes did not return %q", unattachedName)
	}
	if unattached.Mounted {
		t.Errorf("%q reported Mounted=true, want false", unattachedName)
	}

	mounted, ok := byName[mountedName]
	if !ok {
		t.Fatalf("ListNamedVolumes did not return %q", mountedName)
	}
	if !mounted.Mounted {
		t.Errorf("%q reported Mounted=false, want true: it's attached to a running container", mountedName)
	}

	if _, ok := byName[anonResp.Name]; ok {
		t.Errorf("ListNamedVolumes returned the anonymous volume %q, want it excluded", anonResp.Name)
	}
}

// TestClient_ListNamedVolumes_Live_ScopesToOwnInstanceLabel proves the
// exact-match instance-label discipline this package's own doc comment
// promises: a Client built WithInstanceLabel never returns another
// instance's project-named volume, and never an unlabeled one either,
// mirroring application.NetworkCleanupController.ownNetworks' own
// tightened behavior (see PR #557) applied here from the start.
func TestClient_ListNamedVolumes_Live_ScopesToOwnInstanceLabel(t *testing.T) {
	if testing.Short() {
		t.Skip("flaky under concurrent Docker daemon load on shared CI runners, covered by the nightly full suite")
	}
	base := liveClient(t)
	ctx := context.Background()

	scoped, err := NewClient(WithInstanceLabel(spec.InstanceLabelKey, "instance-a"))
	if err != nil {
		t.Fatalf("new scoped client: %v", err)
	}
	t.Cleanup(func() { _ = scoped.Close() })

	ownName := "app-levelrail-test-orphan-scope-own-data"
	if err := scoped.EnsureVolume(ctx, ownName); err != nil {
		t.Fatalf("ensure own volume: %v", err)
	}
	t.Cleanup(func() { _ = base.cli.VolumeRemove(context.Background(), ownName, true) })

	otherName := "app-levelrail-test-orphan-scope-other-data"
	if _, err := base.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   otherName,
		Labels: map[string]string{spec.InstanceLabelKey: "instance-b"},
	}); err != nil {
		t.Fatalf("create other-instance volume: %v", err)
	}
	t.Cleanup(func() { _ = base.cli.VolumeRemove(context.Background(), otherName, true) })

	unlabeledName := "app-levelrail-test-orphan-scope-unlabeled-data"
	if err := base.EnsureVolume(ctx, unlabeledName); err != nil { // base has no instance label configured
		t.Fatalf("create unlabeled volume: %v", err)
	}
	t.Cleanup(func() { _ = base.cli.VolumeRemove(context.Background(), unlabeledName, true) })

	vols, err := scoped.ListNamedVolumes(ctx)
	if err != nil {
		t.Fatalf("ListNamedVolumes: %v", err)
	}
	byName := make(map[string]bool, len(vols))
	for _, v := range vols {
		byName[v.Name] = true
	}
	if !byName[ownName] {
		t.Errorf("scoped client did not return its own volume %q", ownName)
	}
	if byName[otherName] {
		t.Errorf("scoped client returned another instance's volume %q, want it excluded", otherName)
	}
	if byName[unlabeledName] {
		t.Errorf("scoped client returned an unlabeled volume %q, want it excluded (exact-match-only discipline)", unlabeledName)
	}
}

// TestClient_RemoveVolume_Live proves RemoveVolume actually deletes the
// named volume from the daemon.
func TestClient_RemoveVolume_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("flaky under concurrent Docker daemon load on shared CI runners, covered by the nightly full suite")
	}
	c := liveClient(t)
	ctx := context.Background()

	name := "app-levelrail-test-orphan-remove-data"
	if err := c.EnsureVolume(ctx, name); err != nil {
		t.Fatalf("ensure volume: %v", err)
	}

	if err := c.RemoveVolume(ctx, name); err != nil {
		t.Fatalf("RemoveVolume: %v", err)
	}

	vols, err := c.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("list volumes after remove: %v", err)
	}
	for _, v := range vols.Volumes {
		if v.Name == name {
			t.Errorf("volume %q still exists after RemoveVolume", name)
		}
	}
}
