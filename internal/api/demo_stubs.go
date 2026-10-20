package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// demoDoctorReport is the fixed doctor result the demo serves instead of
// probing the host. It agrees with the seeded estate: Docker answers, the
// database is reachable, and the worker disk sits at 82 percent, the same
// fact the firing disk alert reports.
func demoDoctorReport() systemDoctorResponse {
	ok := func(code, name, msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
	}
	disk := doctorCheckResource{
		Code: "disk_space", Name: "Disk space", Status: doctorStatusWarn,
		Message: "14.4 GiB free of 80 GiB on worker-sgp-1, below the 20 percent warning threshold",
		Fix:     "docker image prune -a --filter until=168h", DocsPath: "/troubleshooting#disk-space", Class: "recommendation",
	}
	return systemDoctorResponse{OK: true, Checks: []doctorCheckResource{
		ok("docker", "Docker daemon", "reachable"),
		ok("container_runtime", "Container runtime", "docker engine, rootful"),
		disk,
		ok("data_dir_writable", "Data directory writable", "writable"),
		ok("port_80", "Port 80 available", "in use by this control plane's own ingress, as expected"),
		ok("port_443", "Port 443 available", "in use by this control plane's own ingress, as expected"),
		ok("database", "Database", "reachable"),
		ok("tls_certificates", "TLS certificates", "all certificates valid for at least 30 days"),
	}}
}

// writeDemoUpdates answers GET /api/v1/updates with no release lookup: the
// demo never reaches the internet, so it always reports up to date.
func writeDemoUpdates(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, updateStatusResource{
		CurrentVersion: version.Version, ReleaseKind: upgrade.KindRelease, Channel: upgrade.ChannelStable,
	})
}

// writeDemoDNSHealthChecks answers GET /api/v1/dns/health-checks with an empty
// list: no DNS provider is connected in the demo.
func writeDemoDNSHealthChecks(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, []struct{}{})
}
