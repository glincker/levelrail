package api

import (
	"net/http"
	"syscall"
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
		resp.DockerConnected = rt.dockerPinger.Ping(r.Context()) == nil
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
