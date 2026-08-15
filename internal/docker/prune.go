package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
)

// This file adds manual maintenance operations (disk usage reporting
// and cleanup) on top of the Docker Engine API's own storage accounting
// and prune endpoints. It is deliberately conservative: this platform
// builds and redeploys images constantly (every deploy leaves the
// previous image, and once superseded, the previous container, behind
// on purpose), so an operator's "clean up now" action has real,
// permanent consequences if it guesses wrong about what's actually safe
// to remove.
//
// Every method here is intentionally narrower than what the raw Docker
// Engine API prune endpoints (ContainersPrune, ImagesPrune,
// VolumesPrune) would do by default, for two independent reasons:
//
//  1. This project's own rollback design keeps the previous N images
//     pinned by tag so garbage collection cannot orphan a rollback
//     target (levelrail's own architecture notes on Phase 1 rollback:
//     "keep the previous N images pinned so garbage collection cannot
//     orphan a rollback target"). An aggressive "remove every image not
//     backing a container right now" prune would silently defeat that
//     guarantee the moment an operator clicks a button, so image
//     pruning here only ever touches dangling (untagged, unreferenced)
//     images, never a tagged one, regardless of whether that tagged
//     image currently backs a running container.
//
//  2. internal/reconcile/application.Controller's own ensureReplicaRunning
//     restarts a crashed container in place by ID
//     (c.runtime.Start(ctx, state.ID)) rather than always recreating
//     one. That means a stopped container is not automatically garbage:
//     it can be exactly the container the reconciler is about to
//     restart the next time it runs. The Docker Engine API's own bulk
//     ContainersPrune has no way to exclude a specific container by
//     name (only by label or creation time), so this package never
//     calls it directly for containers; PruneContainers below does the
//     list-filter-remove sequence itself so a caller-supplied "keep"
//     set can protect exactly the containers that are still desired,
//     just not currently running.
//
// Named Docker volumes get an even simpler guarantee: this project's
// own reconcilers (EnsureVolume in client.go; dataVolumeName in
// internal/reconcile/database/controller.go) never create an unnamed
// volume, so restricting volume pruning to Docker's own anonymous
// (64-hex-character, daemon-generated) naming convention means a
// database's data volume can never be a candidate, independent of
// whether it currently looks "in use."
//
// keep-set computation (which container names are currently desired)
// deliberately does not live in this package: internal/docker is kept
// spec- and store-agnostic, the same convention ContainerSpec's own doc
// comment already establishes, so that responsibility belongs to
// internal/api, which has access to the store.

// DiskUsage is Docker's own storage accounting: space claimed by
// images, containers, volumes, and BuildKit's build cache (BuildKit
// runs embedded inside this daemon, see internal/build/client.go's own
// doc comment, so this daemon's build-cache accounting is the real
// number for this project's own builds too, not an unrelated cache).
//
// This is a materially different number from GET /api/v1/system/status's
// existing DataDirTotalBytes/DataDirFreeBytes fields: those measure the
// control plane's own APP_DATA_DIR (app source checkouts, telemetry
// storage, the SQLite file), a plain host filesystem path with no
// relationship to where Docker itself stores image layers, container
// writable layers, or named volumes (typically under Docker's own
// data-root, e.g. /var/lib/docker). An operator whose data dir looks
// healthy can still be moments from a full disk because of accumulated
// Docker state, which is exactly the gap this reports.
type DiskUsage struct {
	ImagesTotalBytes int64
	// ImagesReclaimableBytes follows Docker's own `docker system df`
	// methodology (unused = zero containers reference the image,
	// tagged or not), so an operator sees the same number the docker
	// CLI would show them directly on this host. It is deliberately a
	// bigger number than what PruneDanglingImages actually frees: that
	// call only ever removes the dangling (untagged) subset, on
	// purpose, see PruneDanglingImages' own doc comment for why a
	// tagged-but-currently-unused image is a real rollback target this
	// project has an explicit design rule to protect.
	ImagesReclaimableBytes int64

	ContainersTotalBytes int64
	// ContainersReclaimableBytes sums the writable-layer size (SizeRw,
	// not SizeRootFs, which would double-count shared image layers) of
	// every non-running container, matching what PruneContainers is
	// eligible to remove before its own additional "currently desired"
	// exclusion is applied.
	ContainersReclaimableBytes int64

	VolumesTotalBytes int64
	// VolumesReclaimableBytes sums every volume Docker itself reports
	// as unreferenced by any container (RefCount == 0), a bigger set
	// than PruneAnonymousVolumes actually touches: that call only
	// removes the anonymous subset, never a named volume regardless of
	// use, because a volume that looks unused can be one mid-recreate
	// during a database engine version bump, see
	// PruneAnonymousVolumes' own doc comment.
	VolumesReclaimableBytes int64

	BuildCacheTotalBytes int64
	// BuildCacheReclaimableBytes sums every build cache record not
	// currently InUse, matching what PruneBuildCache removes.
	BuildCacheReclaimableBytes int64
}

// DiskUsage reports Docker's own storage accounting via GET /system/df,
// aggregated into the four categories the General settings page shows.
// See the DiskUsage type's own doc comment for why this is a different
// number from the control plane's own DataDir usage.
func (c *Client) DiskUsage(ctx context.Context) (DiskUsage, error) {
	du, err := c.cli.DiskUsage(ctx, dockertypes.DiskUsageOptions{})
	if err != nil {
		return DiskUsage{}, fmt.Errorf("docker: disk usage: %w", err)
	}
	return aggregateDiskUsage(du), nil
}

// aggregateDiskUsage is DiskUsage's pure aggregation step, split out so
// it can be unit tested against a hand-built dockertypes.DiskUsage
// without a real daemon.
func aggregateDiskUsage(du dockertypes.DiskUsage) DiskUsage {
	var out DiskUsage
	for _, img := range du.Images {
		if img == nil {
			continue
		}
		out.ImagesTotalBytes += img.Size
		if img.Containers == 0 {
			out.ImagesReclaimableBytes += img.Size
		}
	}
	for _, ctr := range du.Containers {
		if ctr == nil {
			continue
		}
		out.ContainersTotalBytes += ctr.SizeRw
		if ctr.State != "running" {
			out.ContainersReclaimableBytes += ctr.SizeRw
		}
	}
	for _, vol := range du.Volumes {
		size := volumeSize(vol)
		if size < 0 {
			continue // "not available" for this volume driver, not zero
		}
		out.VolumesTotalBytes += size
		if vol.UsageData != nil && vol.UsageData.RefCount == 0 {
			out.VolumesReclaimableBytes += size
		}
	}
	for _, bc := range du.BuildCache {
		if bc == nil {
			continue
		}
		out.BuildCacheTotalBytes += bc.Size
		if !bc.InUse {
			out.BuildCacheReclaimableBytes += bc.Size
		}
	}
	return out
}

// volumeSize returns v's on-disk size, or -1 if unknown/unavailable
// (nil UsageData, or a volume driver that doesn't report size, which
// the Engine API also signals as a negative UsageData.Size).
func volumeSize(v *volume.Volume) int64 {
	if v == nil || v.UsageData == nil {
		return -1
	}
	return v.UsageData.Size
}

// PruneContainersResult is one PruneContainers call's outcome.
type PruneContainersResult struct {
	Removed        []string
	ReclaimedBytes uint64
}

// hexID64RE matches Docker's own anonymous-volume naming convention: a
// 64-character lowercase hex string, the same content-addressable-style
// random ID Docker assigns when a container mounts a volume without a
// name (e.g. a Dockerfile's own VOLUME instruction with no matching
// app.yaml volume declaration). A caller-named volume, the only kind
// this project's own reconciler ever creates, never matches this
// pattern; see this file's own package doc comment for why that makes
// this the deciding safety gate for PruneAnonymousVolumes.
var hexID64RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// isAnonymousVolumeName reports whether name is a Docker-generated
// anonymous volume ID rather than one an operator or this project's own
// reconciler chose.
func isAnonymousVolumeName(name string) bool {
	return hexID64RE.MatchString(name)
}

// stoppedNotKept filters summaries (every currently non-running
// container this daemon reports, as PruneContainers already restricted
// its ContainerList call to) down to the ones actually eligible for
// removal: not exactly named in keep. Split out from PruneContainers so
// this selection logic, the same keepSet-lookup shape
// internal/reconcile/application.Controller.staleContainers already
// uses for its own stale-container check, can be unit tested without a
// real daemon.
func stoppedNotKept(summaries []dockertypes.Container, keep []string) []dockertypes.Container {
	keepSet := make(map[string]bool, len(keep))
	for _, name := range keep {
		keepSet[name] = true
	}
	var out []dockertypes.Container
	for _, cs := range summaries {
		if keepSet[containerSummaryName(cs)] {
			continue
		}
		out = append(out, cs)
	}
	return out
}

func containerSummaryName(cs dockertypes.Container) string {
	if len(cs.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(cs.Names[0], "/")
}

// PruneContainers stops and removes every non-running container on this
// daemon whose name is not in keep, using a direct list-then-remove
// sequence (each already-stopped container removed individually) rather
// than the Docker Engine API's own bulk POST /containers/prune
// (ContainersPrune in the SDK). See this file's own package doc comment
// for why: that bulk endpoint cannot exclude specific containers by
// name, and this project's reconciler can consider a stopped container
// still desired (about to be restarted in place). keep must include
// every container name the caller currently considers live, e.g. every
// application replica's target container name plus every database's
// container name, or this can race a reconcile pass that's about to
// restart one of them.
//
// Only containers in Docker's "exited" or "created" state are
// candidates, matching the scope Docker's own bulk container prune
// targets. "paused" and "restarting" are transient, mid-lifecycle
// states, never a stable "this is garbage" signal, so they're left
// alone entirely; "dead", a rare failure state of Docker's own
// container teardown, is left for an operator to investigate rather
// than silently swept up.
//
// Since every candidate is already confirmed non-running by the status
// filter, removal never needs Force: true (that flag exists to stop a
// running container before removing it, not relevant here) and never
// sets RemoveVolumes: cascading volume deletion through container
// removal would bypass PruneAnonymousVolumes' own, more careful safety
// checks; volume cleanup is that method's job alone.
//
// One narrow, self-healing race remains, worth naming rather than
// treating as impossible: internal/reconcile/application's
// createAndStart calls Create then Start as two separate steps, so a
// container briefly exists in Docker's own "created" (not yet running)
// state between them. The candidate filter above (status: exited or
// created) matches that window on purpose, since a container that never
// got past Create after a genuine failure is real cruft this should
// still remove. But if the caller's own keep set was computed before
// that exact container's name existed (a deploy landing in the same
// instant a manual prune call is already mid-flight), this could remove
// it. Level-triggered reconcile recovers on its own next pass (one
// wasted create, a logged error, no data loss), the same tolerance this
// codebase already extends to other narrow eventual-consistency windows
// (see internal/docker/client_live_test.go's waitForStopped), so this
// is accepted rather than solved with, e.g., a lock this package has no
// good way to hold across a caller-supplied keep computation anyway.
func (c *Client) PruneContainers(ctx context.Context, keep []string) (PruneContainersResult, error) {
	f := filters.NewArgs()
	f.Add("status", "exited")
	f.Add("status", "created")
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Size: true, Filters: f})
	if err != nil {
		return PruneContainersResult{}, fmt.Errorf("docker: list stopped containers: %w", err)
	}

	var result PruneContainersResult
	var firstErr error
	for _, cs := range stoppedNotKept(summaries, keep) {
		if err := c.cli.ContainerRemove(ctx, cs.ID, container.RemoveOptions{}); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove stopped container %s: %w", containerSummaryName(cs), err)
			}
			continue
		}
		result.Removed = append(result.Removed, containerSummaryName(cs))
		if cs.SizeRw > 0 {
			result.ReclaimedBytes += uint64(cs.SizeRw) //nolint:gosec // guarded non-negative above
		}
	}
	return result, firstErr
}

// PruneImagesResult is one PruneDanglingImages call's outcome.
type PruneImagesResult struct {
	Removed        []string
	ReclaimedBytes uint64
}

// PruneDanglingImages removes only dangling images: untagged and not
// referenced by any container, Docker's own conservative default for
// `docker image prune` (no -a flag). The "dangling" filter is set
// explicitly here rather than relying on ImagesPrune's behavior with no
// filter at all, so this scope is guaranteed regardless of daemon
// version.
//
// This deliberately never touches a tagged image, even one currently
// unreferenced by any container: this project's own rollback design
// keeps the previous N images pinned by tag so garbage collection
// cannot orphan a rollback target, and a tagged image sitting unused
// right now is exactly what that design depends on staying in place.
// The more aggressive "everything unused, tagged or not" cleanup
// `docker image prune -a` offers has no equivalent here, on purpose.
func (c *Client) PruneDanglingImages(ctx context.Context) (PruneImagesResult, error) {
	f := filters.NewArgs()
	f.Add("dangling", "true")
	report, err := c.cli.ImagesPrune(ctx, f)
	if err != nil {
		return PruneImagesResult{}, fmt.Errorf("docker: prune dangling images: %w", err)
	}
	removed := make([]string, 0, len(report.ImagesDeleted))
	for _, d := range report.ImagesDeleted {
		switch {
		case d.Deleted != "":
			removed = append(removed, d.Deleted)
		case d.Untagged != "":
			removed = append(removed, d.Untagged)
		}
	}
	return PruneImagesResult{Removed: removed, ReclaimedBytes: report.SpaceReclaimed}, nil
}

// PruneVolumesResult is one PruneAnonymousVolumes call's outcome.
type PruneVolumesResult struct {
	Removed        []string
	ReclaimedBytes uint64
}

// PruneAnonymousVolumes removes only anonymous volumes (Docker's own
// randomly-generated 64-character hex name, assigned when a container
// mounts a volume without one, e.g. a Dockerfile's own VOLUME
// instruction with no matching app.yaml volume declaration) that are
// not currently attached to any container, running or stopped.
//
// This project's own EnsureVolume always creates a caller-named volume;
// internal/reconcile/database's dataVolumeName ("db-<name>-data") is the
// only volume name this project's reconciler itself ever produces. So
// restricting to the anonymous-name pattern is not merely mirroring the
// Docker CLI's own conservative default (`docker volume prune` without
// --all only removes anonymous volumes as of Engine API >= 1.42): it is
// a second, independent, daemon-version-proof guarantee that this
// method can never remove a database's data volume, because no name
// this project creates can ever match the anonymous pattern. Named
// volumes, even ones that look unused right now, are always left alone;
// a named volume can look briefly unattached mid-replace during a
// database engine version bump
// (internal/reconcile/database.Controller.replaceContainer: stop,
// remove, then create, strictly sequential) without actually being
// abandoned.
//
// "Not currently attached to any container" is computed from a fresh
// ContainerList across every container, running or stopped, rather than
// trusting the Engine API's own dangling-volume filter, whose exact
// semantics (does it count a stopped container's mounts as
// still-attached?) are not something this method's safety should depend
// on being right.
func (c *Client) PruneAnonymousVolumes(ctx context.Context) (PruneVolumesResult, error) {
	inUse, err := c.mountedVolumeNames(ctx)
	if err != nil {
		return PruneVolumesResult{}, err
	}

	vols, err := c.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return PruneVolumesResult{}, fmt.Errorf("docker: list volumes: %w", err)
	}

	var result PruneVolumesResult
	var firstErr error
	for _, v := range vols.Volumes {
		if v == nil || !isAnonymousVolumeName(v.Name) || inUse[v.Name] {
			continue
		}
		size := volumeSize(v)
		if err := c.cli.VolumeRemove(ctx, v.Name, false); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove volume %s: %w", v.Name, err)
			}
			continue
		}
		result.Removed = append(result.Removed, v.Name)
		if size > 0 {
			result.ReclaimedBytes += uint64(size)
		}
	}
	return result, firstErr
}

// mountedVolumeNames returns the set of volume names referenced by any
// container's mounts, in any container state (running, stopped,
// created): the authoritative "is this volume actually attached to
// something" answer PruneAnonymousVolumes needs, independent of
// whichever unused/dangling accounting the Engine API's own volume
// endpoints happen to apply server-side.
func (c *Client) mountedVolumeNames(ctx context.Context) (map[string]bool, error) {
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers for volume usage: %w", err)
	}
	inUse := make(map[string]bool)
	for _, cs := range summaries {
		for _, m := range cs.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" {
				inUse[m.Name] = true
			}
		}
	}
	return inUse, nil
}

// PruneBuildCacheResult is one PruneBuildCache call's outcome.
type PruneBuildCacheResult struct {
	Removed        []string
	ReclaimedBytes uint64
}

// PruneBuildCache removes unused BuildKit cache records from the
// daemon's embedded BuildKit instance, the same one internal/build's own
// Client drives (internal/build/client.go's own doc comment: BuildKit
// runs inside dockerd, reachable only via this same *dockerclient.Client's
// own /grpc hijack, so this daemon's build-cache accounting is this
// project's real build cache, not an unrelated one). All: false, matching
// `docker builder prune`'s own default, removes only cache not currently
// InUse: a cache record still backing an in-flight build is left alone
// rather than this call racing a build that happens to be running at the
// same moment an operator clicks "clean up now."
func (c *Client) PruneBuildCache(ctx context.Context) (PruneBuildCacheResult, error) {
	report, err := c.cli.BuildCachePrune(ctx, dockertypes.BuildCachePruneOptions{All: false})
	if err != nil {
		return PruneBuildCacheResult{}, fmt.Errorf("docker: prune build cache: %w", err)
	}
	if report == nil {
		return PruneBuildCacheResult{}, nil
	}
	return PruneBuildCacheResult{Removed: report.CachesDeleted, ReclaimedBytes: report.SpaceReclaimed}, nil
}

// PruneResult is what one Prune call actually did, returned as a single
// aggregate report so a caller (handleSystemPrune) can show an operator
// exactly what was removed, not just "done."
type PruneResult struct {
	ContainersRemoved        []string
	ContainersReclaimedBytes uint64
	ImagesRemoved            []string
	ImagesReclaimedBytes     uint64
	VolumesRemoved           []string
	VolumesReclaimedBytes    uint64
	BuildCacheRemoved        []string
	BuildCacheReclaimedBytes uint64
	// Errors collects any stage's failure without aborting the rest:
	// the same "one broken resource must not block others" principle
	// internal/reconcile/application.Controller.removeContainers
	// already applies to stale-container cleanup, applied here across
	// prune stages instead of across containers.
	Errors []string
}

// Prune runs every cleanup stage this package supports, in order:
// stopped containers not in keep, dangling images, anonymous unused
// volumes, then unused build cache. Each stage is independent; a
// failure in one does not stop the others, and every stage's own
// failure (if any) is collected into Errors rather than aborting the
// call, so a caller always gets back whatever the other stages did
// manage to clean up.
//
// keep is every container name the caller currently considers desired
// (every application replica's target container name plus every
// database's container name); see PruneContainers' own doc comment for
// exactly why this cannot be computed inside this package.
func (c *Client) Prune(ctx context.Context, keep []string) PruneResult {
	var result PruneResult

	// containers is used regardless of err: PruneContainers keeps going
	// past an individual container's own removal failure (its own doc
	// comment), so a non-nil err there still carries every container
	// that did succeed before the one that didn't.
	containers, err := c.PruneContainers(ctx, keep)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("containers: %v", err))
	}
	result.ContainersRemoved = containers.Removed
	result.ContainersReclaimedBytes = containers.ReclaimedBytes

	if images, err := c.PruneDanglingImages(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("images: %v", err))
	} else {
		result.ImagesRemoved = images.Removed
		result.ImagesReclaimedBytes = images.ReclaimedBytes
	}

	if volumes, err := c.PruneAnonymousVolumes(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("volumes: %v", err))
	} else {
		result.VolumesRemoved = volumes.Removed
		result.VolumesReclaimedBytes = volumes.ReclaimedBytes
	}

	if buildCache, err := c.PruneBuildCache(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("build cache: %v", err))
	} else {
		result.BuildCacheRemoved = buildCache.Removed
		result.BuildCacheReclaimedBytes = buildCache.ReclaimedBytes
	}

	return result
}
