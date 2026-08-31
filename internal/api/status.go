package api

import (
	"net/http"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// systemStatusResponse is GET /api/v1/system/status's wire shape: a
// small set of honest configured/not-configured signals for the
// General settings page, replacing the "nothing here yet" placeholder
// that page's own comment previously documented. DockerConnected covers
// daemon connectivity (see DockerPinger and its doc comment on why this
// isn't a docker.Runtime method); no version string yet (no build-time
// version variable exists anywhere in this codebase yet), a real,
// separate follow-up, not faked here.
type systemStatusResponse struct {
	SecretsConfigured   bool `json:"secrets_configured"`
	TelemetryConfigured bool `json:"telemetry_configured"`
	AlertsConfigured    bool `json:"alerts_configured"`
	// DataDirTotalBytes/DataDirFreeBytes are 0 (omitted) when no
	// WithDataDir was supplied at construction, or when statfs on that
	// path fails for any reason: a disk-usage read failure degrades to
	// "not reported," the same "optional signal, never blocks the
	// response" shape this whole endpoint follows.
	DataDirTotalBytes int64 `json:"data_dir_total_bytes,omitempty"`
	DataDirFreeBytes  int64 `json:"data_dir_free_bytes,omitempty"`
	// DockerConnected is true only when a DockerPinger is configured
	// (WithDockerPinger) AND its Ping call returns nil. Every other
	// case, no DockerPinger configured at all, or Ping returning an
	// error, degrades to false rather than surfacing as a request
	// error: the same "absence, or a failed optional check, is a real
	// reportable false, never a 5xx" shape SecretsConfigured/
	// TelemetryConfigured/AlertsConfigured already follow above.
	DockerConnected bool `json:"docker_connected"`
	// DockerError carries the Ping failure's own message when
	// DockerConnected is false because Ping errored (never set for the
	// "no DockerPinger configured" case): the raw daemon-unreachable text
	// an operator needs to actually fix the problem, not just "false".
	DockerError string `json:"docker_error,omitempty"`
	// DockerDiskUsage is Docker's own storage accounting (images,
	// containers, volumes, build cache), omitted entirely when no
	// DockerDiskUsager is configured (WithDockerDiskUsager) or the
	// underlying DiskUsage call fails: the same "absence, or a failed
	// optional check, is a real reportable omission, never a 5xx"
	// shape DockerConnected above already follows. See
	// dockerDiskUsageResource's own doc comment for why this is a
	// materially different number from DataDirTotalBytes/
	// DataDirFreeBytes above: those measure this control plane's own
	// APP_DATA_DIR, not Docker's storage.
	DockerDiskUsage *dockerDiskUsageResource `json:"docker_disk_usage,omitempty"`
}

// dockerDiskUsageResource is docker.DiskUsage's wire shape: an explicit
// snake_case translation, the same "don't let an internal/docker Go
// struct's own field names leak onto the wire" convention
// internal/api/images.go's imageResource/toImageResource already
// establishes for docker.ImageInfo, rather than marshaling
// docker.DiskUsage directly.
type dockerDiskUsageResource struct {
	ImagesTotalBytes           int64 `json:"images_total_bytes"`
	ImagesReclaimableBytes     int64 `json:"images_reclaimable_bytes"`
	ContainersTotalBytes       int64 `json:"containers_total_bytes"`
	ContainersReclaimableBytes int64 `json:"containers_reclaimable_bytes"`
	VolumesTotalBytes          int64 `json:"volumes_total_bytes"`
	VolumesReclaimableBytes    int64 `json:"volumes_reclaimable_bytes"`
	BuildCacheTotalBytes       int64 `json:"build_cache_total_bytes"`
	BuildCacheReclaimableBytes int64 `json:"build_cache_reclaimable_bytes"`
}

func toDockerDiskUsageResource(u docker.DiskUsage) *dockerDiskUsageResource {
	return &dockerDiskUsageResource{
		ImagesTotalBytes:           u.ImagesTotalBytes,
		ImagesReclaimableBytes:     u.ImagesReclaimableBytes,
		ContainersTotalBytes:       u.ContainersTotalBytes,
		ContainersReclaimableBytes: u.ContainersReclaimableBytes,
		VolumesTotalBytes:          u.VolumesTotalBytes,
		VolumesReclaimableBytes:    u.VolumesReclaimableBytes,
		BuildCacheTotalBytes:       u.BuildCacheTotalBytes,
		BuildCacheReclaimableBytes: u.BuildCacheReclaimableBytes,
	}
}

// handleSystemStatus handles GET /api/v1/system/status. Always 200: an
// unconfigured optional feature is a real, reportable false, not an
// error condition, the same reasoning that keeps handleDevMode public
// and unconditional.
func (rt *Router) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	resp := systemStatusResponse{
		SecretsConfigured:   rt.secrets != nil,
		TelemetryConfigured: rt.telemetry != nil,
		AlertsConfigured:    rt.alertRules != nil,
	}

	if rt.dockerPinger != nil {
		if err := rt.dockerPinger.Ping(r.Context()); err != nil {
			resp.DockerError = err.Error()
		} else {
			resp.DockerConnected = true
		}
	}

	if rt.dockerDiskUsage != nil {
		if usage, err := rt.dockerDiskUsage.DiskUsage(r.Context()); err == nil {
			resp.DockerDiskUsage = toDockerDiskUsageResource(usage)
		}
		// An error here degrades to "omitted," the same as DockerConnected
		// above on a failed Ping: a daemon hiccup on this one read must
		// never turn the whole status endpoint into a 500.
	}

	if rt.dataDir != "" {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(rt.dataDir, &stat); err == nil {
			// Bavail (blocks available to an unprivileged user) is the
			// honest "free" number to show an operator deciding whether
			// they're about to run out of room, not Bfree (includes
			// root-reserved blocks).
			resp.DataDirTotalBytes = int64(stat.Blocks) * int64(stat.Bsize) //nolint:gosec // statfs fields are always non-negative in practice
			resp.DataDirFreeBytes = int64(stat.Bavail) * int64(stat.Bsize)  //nolint:gosec // statfs fields are always non-negative in practice
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
