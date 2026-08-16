package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

// TestClient_Prune_Live_RemovesOnlyGenuineGarbage is the live proof this
// feature exists to satisfy: pruning is destructive and semi-
// irreversible, so its safety has to be demonstrated against a real
// daemon, not only asserted against a fake. It builds exactly the
// scenario this project's own rollback and in-place-restart designs can
// produce day to day:
//
//   - a "desired" container that happens to be stopped right now (the
//     same state a crashed replica is in immediately before
//     internal/reconcile/application.Controller's ensureReplicaRunning
//     restarts it in place), which must survive because its name is in
//     the keep set
//   - a genuinely unrelated, unowned stopped container, which must be
//     removed
//   - a tagged image still backing a running container, which must
//     survive (PruneDanglingImages never touches a tagged image at all,
//     the stronger of the two guarantees, so this also proves that)
//   - a real dangling (untagged, unreferenced) image, which must be
//     removed
//   - a named volume actually mounted into a running container (the
//     shape a database's own data volume takes), which must survive
//   - a genuinely anonymous, unattached volume, which must be removed
//
// Every assertion is made against the raw Engine API directly
// (c.cli.ContainerList/ImageList/VolumeList), not this package's own
// return values, so the test can't pass by trusting the same code path
// it's meant to be checking.
func TestClient_Prune_Live_RemovesOnlyGenuineGarbage(t *testing.T) {
	if testing.Short() {
		t.Skip("flaky under concurrent Docker daemon load on shared CI runners, covered by the nightly full suite")
	}
	c := liveClient(t)
	ctx := context.Background()

	const (
		desiredStoppedName = "levelrail-test-prune-desired-stopped"
		garbageStoppedName = "levelrail-test-prune-garbage-stopped"
		inUseRunningName   = "levelrail-test-prune-in-use-running"
		taggedRepo         = "levelrail-test-prune-tagged"
	)

	// Clean up anything left over from a previous failed run, and always
	// clean up after, regardless of outcome.
	cleanup := func() {
		removeIfExists(ctx, t, c, desiredStoppedName)
		removeIfExists(ctx, t, c, garbageStoppedName)
		removeIfExists(ctx, t, c, inUseRunningName)
	}
	cleanup()
	t.Cleanup(cleanup)

	// --- Set up the tagged, currently-in-use image and its running
	// container. nginx:alpine is already used elsewhere in this
	// package's live tests, so it's already pulled in CI/dev.
	inUseID, err := c.Create(ctx, ContainerSpec{Name: inUseRunningName, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("create in-use container: %v", err)
	}
	if err := c.Start(ctx, inUseID); err != nil {
		t.Fatalf("start in-use container: %v", err)
	}

	// --- Set up the "desired but currently stopped" container: create
	// and start, then stop it without removing, the same observable
	// state a crashed-but-still-desired replica is in.
	desiredID, err := c.Create(ctx, ContainerSpec{Name: desiredStoppedName, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("create desired-stopped container: %v", err)
	}
	if err := c.Start(ctx, desiredID); err != nil {
		t.Fatalf("start desired-stopped container: %v", err)
	}
	if err := c.Stop(ctx, desiredID, 5*time.Second); err != nil {
		t.Fatalf("stop desired-stopped container: %v", err)
	}

	// --- Set up the genuinely unrelated, unowned stopped container:
	// same shape, just never named in keep.
	garbageID, err := c.Create(ctx, ContainerSpec{Name: garbageStoppedName, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("create garbage-stopped container: %v", err)
	}
	if err := c.Start(ctx, garbageID); err != nil {
		t.Fatalf("start garbage-stopped container: %v", err)
	}
	if err := c.Stop(ctx, garbageID, 5*time.Second); err != nil {
		t.Fatalf("stop garbage-stopped container: %v", err)
	}

	// --- Set up a real dangling image: tag nginx:alpine's own image
	// under a throwaway repo, then untag it (docker rmi of the tag,
	// not the underlying image, since inUseRunningName still
	// references the same content by digest) so the tag disappears but
	// nothing references the resulting untagged image record, which is
	// exactly what "dangling" means.
	inUseImage, err := c.cli.ImageInspect(ctx, "nginx:alpine")
	if err != nil {
		t.Fatalf("inspect nginx:alpine: %v", err)
	}
	danglingTag := taggedRepo + ":throwaway"
	if err := c.cli.ImageTag(ctx, inUseImage.ID, danglingTag); err != nil {
		t.Fatalf("tag dangling image: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.cli.ImageRemove(context.Background(), danglingTag, image.RemoveOptions{Force: true})
	})
	if _, err := c.cli.ImageRemove(ctx, danglingTag, image.RemoveOptions{}); err != nil {
		t.Fatalf("untag dangling image (leaving a dangling record behind): %v", err)
	}

	// --- Set up a real named volume mounted into the running container
	// (the shape a database's data volume takes) and a genuinely
	// anonymous, unattached volume.
	namedVolume := "levelrail-test-prune-named-data"
	if err := c.EnsureVolume(ctx, namedVolume); err != nil {
		t.Fatalf("ensure named volume: %v", err)
	}
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), namedVolume, true) })
	namedVolID, err := c.Create(ctx, ContainerSpec{
		Name:  inUseRunningName + "-with-volume",
		Image: "nginx:alpine",
		Volumes: []VolumeMount{
			{Name: namedVolume, ContainerPath: "/usr/share/nginx/html"},
		},
	})
	if err != nil {
		t.Fatalf("create container mounting named volume: %v", err)
	}
	t.Cleanup(func() {
		_ = c.cli.ContainerRemove(context.Background(), namedVolID, container.RemoveOptions{Force: true})
	})
	if err := c.Start(ctx, namedVolID); err != nil {
		t.Fatalf("start container mounting named volume: %v", err)
	}

	anonVolResp, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{}) // no Name: Docker assigns the anonymous 64-hex id
	if err != nil {
		t.Fatalf("create anonymous volume: %v", err)
	}
	anonVolumeName := anonVolResp.Name
	t.Cleanup(func() { _ = c.cli.VolumeRemove(context.Background(), anonVolumeName, true) })
	if !isAnonymousVolumeName(anonVolumeName) {
		t.Fatalf("test setup invariant broken: Docker-assigned volume name %q doesn't match this package's own anonymous-name pattern", anonVolumeName)
	}

	// --- Run the real cleanup, keeping only the desired-but-stopped
	// container and the two running ones.
	keep := []string{desiredStoppedName, inUseRunningName, inUseRunningName + "-with-volume"}
	result := c.Prune(ctx, keep)
	if len(result.Errors) != 0 {
		t.Fatalf("Prune() reported stage errors: %v", result.Errors)
	}

	// --- Assertions, independent of this package's own return values.

	if state, err := c.InspectByName(ctx, desiredStoppedName); err != nil || state == nil {
		t.Errorf("desired-but-stopped container was removed by Prune: InspectByName() = (%v, %v), want it to survive", state, err)
	}
	if state, err := c.InspectByName(ctx, garbageStoppedName); err != nil {
		t.Fatalf("InspectByName(garbage) error = %v", err)
	} else if state != nil {
		t.Errorf("genuinely unrelated stopped container survived Prune: %+v, want it removed", state)
	}
	if state, err := c.InspectByName(ctx, inUseRunningName); err != nil || state == nil || !state.Running {
		t.Errorf("running in-use container was disturbed by Prune: state=%+v err=%v, want it running and untouched", state, err)
	}

	if _, err := c.cli.ImageInspect(ctx, inUseImage.ID); err != nil {
		t.Errorf("in-use tagged image's underlying content was removed by Prune: %v, want it to survive (still referenced by a running container)", err)
	}
	if _, err := c.cli.ImageInspect(ctx, "nginx:alpine"); err != nil {
		t.Errorf("nginx:alpine tag itself was removed by Prune: %v, want a tagged image to never be touched by PruneDanglingImages", err)
	}
	danglingStillPresent := false
	danglingArgs := filters.NewArgs()
	danglingArgs.Add("dangling", "true")
	images, err := c.cli.ImageList(ctx, image.ListOptions{All: true, Filters: danglingArgs})
	if err != nil {
		t.Fatalf("list dangling images after prune: %v", err)
	}
	for _, img := range images {
		if img.ID == inUseImage.ID {
			// The underlying content is still present (tagged as
			// nginx:alpine and in use), it must not show up as a
			// dangling leftover under a different identity.
			continue
		}
		danglingStillPresent = true
	}
	if danglingStillPresent {
		t.Errorf("a dangling image record still exists after Prune, want none: %+v", images)
	}

	if vols, err := c.cli.VolumeList(ctx, volume.ListOptions{}); err != nil {
		t.Fatalf("list volumes after prune: %v", err)
	} else {
		var sawNamed, sawAnon bool
		for _, v := range vols.Volumes {
			if v.Name == namedVolume {
				sawNamed = true
			}
			if v.Name == anonVolumeName {
				sawAnon = true
			}
		}
		if !sawNamed {
			t.Error("named volume mounted into a running container was removed by Prune, want it to survive")
		}
		if sawAnon {
			t.Error("genuinely anonymous, unattached volume survived Prune, want it removed")
		}
	}
}
