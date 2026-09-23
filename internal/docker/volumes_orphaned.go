package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/volume"
)

// NamedVolume is one of this project's own named (non-anonymous) Docker
// volumes, the candidate set GET /api/v1/system/volumes/orphaned filters
// down to "genuinely orphaned" by cross-referencing against current
// desired state. That cross-reference deliberately does not live here:
// see this file's package doc comment in prune.go for why keep-set/
// desired-state computation belongs to internal/api, which has store
// access, not internal/docker.
type NamedVolume struct {
	Name      string
	SizeBytes int64  // -1 if the volume driver doesn't report size
	CreatedAt string // Docker's own volume creation timestamp, RFC3339
	// Mounted reports whether this volume is currently attached to any
	// container, running or stopped, the same mountedVolumeNames signal
	// PruneAnonymousVolumes already relies on. A caller can use this as
	// a second, independent guard on top of desired-state absence: this
	// project's own containers are only ever created from desired state,
	// so a volume neither in desired state nor mounted anywhere is
	// orphaned by two independent signals, not one.
	Mounted bool
}

// namedVolumePrefixes are the only Docker volume name shapes this
// project's own reconcilers ever produce: "app-" (internal/deploy's and
// internal/compose's own volumeName helpers) and "db-"
// (internal/reconcile/database's dataVolumeName/certsVolumeName). Any
// other name, including Docker's own anonymous 64-hex convention
// (isAnonymousVolumeName) and a volume an operator created by hand
// outside this platform entirely, is left out of the orphan-detection
// candidate set: this project can only reason about "orphaned" for a
// volume it can positively trace back to its own naming scheme.
var namedVolumePrefixes = []string{"app-", "db-"}

// isProjectNamedVolume reports whether name matches this project's own
// named-volume convention, the positive-identification gate
// ListNamedVolumes applies before a volume is even considered a
// candidate.
func isProjectNamedVolume(name string) bool {
	for _, prefix := range namedVolumePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ListNamedVolumes returns every Docker volume on this daemon that
// matches this project's own named-volume convention (isProjectNamedVolume)
// and, when this Client was built with WithInstanceLabel, carries this
// exact instance's own label. An unlabeled volume, or one labeled with a
// different instance's ID, is never returned: the same exact-match-only
// discipline application.NetworkCleanupController.ownNetworks was
// tightened to (see that method's own doc comment), applied here from
// the start rather than needing a later fix. EnsureVolume already stamps
// every volume this Client creates with the instance label when one is
// configured, so there is no pre-labeling-upgrade case to accommodate
// the way the network cleanup fix had to: an exact match is always
// achievable for a volume this instance genuinely created.
//
// No instance label configured (single-instance mode, the default) is a
// no-op on this check: every project-named volume is a candidate,
// mirroring PruneAnonymousVolumes' own "empty means no scoping" fallback.
func (c *Client) ListNamedVolumes(ctx context.Context) ([]NamedVolume, error) {
	vols, err := c.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: list volumes: %w", err)
	}
	inUse, err := c.mountedVolumeNames(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]NamedVolume, 0, len(vols.Volumes))
	for _, v := range vols.Volumes {
		if v == nil || !isProjectNamedVolume(v.Name) {
			continue
		}
		if c.hasInstanceLabel() && v.Labels[c.instanceLabelKey] != c.instanceLabelValue {
			continue
		}
		out = append(out, NamedVolume{
			Name:      v.Name,
			SizeBytes: volumeSize(v),
			CreatedAt: v.CreatedAt,
			Mounted:   inUse[v.Name],
		})
	}
	return out, nil
}

// RemoveVolume deletes a single named Docker volume by name. Unlike
// PruneAnonymousVolumes, this takes an exact, caller-supplied name
// rather than discovering candidates itself: the caller
// (internal/api's handleCleanupOrphanedVolumes) has already done its own
// desired-state cross-reference and is asking for one specific,
// operator-confirmed volume, not "whatever currently looks unused."
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	if err := c.cli.VolumeRemove(ctx, name, false); err != nil {
		return fmt.Errorf("docker: remove volume %q: %w", name, err)
	}
	return nil
}
