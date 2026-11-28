package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	dockerclient "github.com/moby/moby/client"

	dockertypes "github.com/moby/moby/api/types"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
)

// This file adds manual maintenance operations (disk usage reporting
// and cleanup) on top of the Docker Engine API's own storage accounting
// and prune endpoints. It is deliberately conservative: this platform.

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
	du, err := c.cli.DiskUsage(ctx, dockerclient.DiskUsageOptions{})
	if err != nil {
		return DiskUsage{}, fmt.Errorf("docker: disk usage: %w", err)
	}
	return aggregateDiskUsage(du), nil
}

// aggregateDiskUsage is DiskUsage's pure aggregation step, split out so
// it can be unit tested against a hand-built dockertypes.DiskUsage
// without a real daemon. The four categories are independent (each
// sums a different du field into different DiskUsage fields), so each
// gets its own small helper below rather than one function summing all
// four in turn.
func aggregateDiskUsage(du dockerclient.DiskUsageResult) DiskUsage {
	return DiskUsage{
		ImagesTotalBytes:           du.Images.TotalSize,
		ImagesReclaimableBytes:     du.Images.Reclaimable,
		ContainersTotalBytes:       du.Containers.TotalSize,
		ContainersReclaimableBytes: du.Containers.Reclaimable,
		VolumesTotalBytes:          du.Volumes.TotalSize,
		VolumesReclaimableBytes:    du.Volumes.Reclaimable,
		BuildCacheTotalBytes:       du.BuildCache.TotalSize,
		BuildCacheReclaimableBytes: du.BuildCache.Reclaimable,
	}
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
func stoppedNotKept(summaries []container.Summary, keep []string) []container.Summary {
	keepSet := make(map[string]bool, len(keep))
	for _, name := range keep {
		keepSet[name] = true
	}
	var out []container.Summary
	for _, cs := range summaries {
		if keepSet[containerSummaryName(cs)] {
			continue
		}
		out = append(out, cs)
	}
	return out
}

func containerSummaryName(cs container.Summary) string {
	if len(cs.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(cs.Names[0], "/")
}

// PruneContainers stops and removes every non-running container on this
// daemon whose name is not in keep, using a direct list-then-remove
// sequence (each already-stopped container removed individually) rather.
func (c *Client) PruneContainers(ctx context.Context, keep []string) (PruneContainersResult, error) {
	f := dockerclient.Filters{}
	f.Add("status", "exited")
	f.Add("status", "created")
	c.instanceLabelFilter(f)
	summaries, err := c.cli.ContainerList(ctx, dockerclient.ContainerListOptions{All: true, Size: true, Filters: f})
	if err != nil {
		return PruneContainersResult{}, fmt.Errorf("docker: list stopped containers: %w", err)
	}

	var result PruneContainersResult
	var firstErr error
	for _, cs := range stoppedNotKept(summaries, keep) {
		if err := c.cli.ContainerRemove(ctx, cs.ID, dockerclient.ContainerRemoveOptions{}); err != nil {
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
// `docker image prune` (no -a flag). The "dangling" filter is set.
func (c *Client) PruneDanglingImages(ctx context.Context) (PruneImagesResult, error) {
	f := dockerclient.Filters{}
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
// mounts a volume without one, e.g. a Dockerfile's own VOLUME.
func (c *Client) PruneAnonymousVolumes(ctx context.Context) (PruneVolumesResult, error) {
	dangling, err := c.ListDanglingAnonymousVolumes(ctx)
	if err != nil {
		return PruneVolumesResult{}, err
	}
	var result PruneVolumesResult
	var firstErr error
	for _, v := range dangling {
		if err := c.cli.VolumeRemove(ctx, v.Name, false); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove volume %s: %w", v.Name, err)
			}
			continue
		}
		result.Removed = append(result.Removed, v.Name)
		if v.SizeBytes > 0 {
			result.ReclaimedBytes += uint64(v.SizeBytes)
		}
	}
	return result, firstErr
}

// AnonymousVolume is an unnamed volume Docker created for an image VOLUME
// directive that no container references any more.
type AnonymousVolume struct {
	Name      string
	SizeBytes int64
}

// ListDanglingAnonymousVolumes is the read-only half of PruneAnonymousVolumes:
// the same selection (anonymous name, mounted by no container in any state),
// never a named volume, so a dry run shows exactly what a prune would remove.
func (c *Client) ListDanglingAnonymousVolumes(ctx context.Context) ([]AnonymousVolume, error) {
	inUse, err := c.mountedVolumeNames(ctx)
	if err != nil {
		return nil, err
	}
	vols, err := c.cli.VolumeList(ctx, dockerclient.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list volumes: %w", err)
	}
	var out []AnonymousVolume
	for _, v := range vols.Volumes {
		if !isAnonymousVolumeName(v.Name) || inUse[v.Name] {
			continue
		}
		out = append(out, AnonymousVolume{Name: v.Name, SizeBytes: volumeSize(&v)})
	}
	return out, nil
}

// mountedVolumeNames returns the set of volume names referenced by any
// container's mounts, in any container state (running, stopped,
// created): the authoritative "is this volume actually attached to
// something" answer PruneAnonymousVolumes needs, independent of
// whichever unused/dangling accounting the Engine API's own volume
// endpoints happen to apply server-side.
func (c *Client) mountedVolumeNames(ctx context.Context) (map[string]bool, error) {
	summaries, err := c.cli.ContainerList(ctx, dockerclient.ContainerListOptions{All: true})
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
//
// Deliberately not scoped by this Client's own instance label: BuildKit
// cache records carry no container/image identity at all to key an
// instance label off of, this project or any other's. The InUse check
// above is what already makes this safe to share a daemon: a build any
// control-plane instance has in flight keeps its cache records InUse
// regardless of which instance started it.
func (c *Client) PruneBuildCache(ctx context.Context) (PruneBuildCacheResult, error) {
	report, err := c.cli.BuildCachePrune(ctx, dockerclient.BuildCachePruneOptions{All: false})
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
