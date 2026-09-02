package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

// Doctor check statuses. Warn never affects the response's overall OK
// field, only fail does: a warning is actionable information, not a
// blocker.
const (
	doctorStatusOK      = "ok"
	doctorStatusWarn    = "warn"
	doctorStatusFail    = "fail"
	doctorStatusUnknown = "unknown"
)

// defaultDoctorDiskWarningBytes is the free-space floor GET
// /api/v1/system/doctor's disk_space check warns below, overridable via
// APP_DOCTOR_DISK_WARNING_BYTES (WithDoctorDiskWarningBytes).
const defaultDoctorDiskWarningBytes = 1 << 30 // 1GiB

// doctorPortCheckTimeout bounds the SQLite ping doctor.go issues: a
// stuck check must never hang the whole doctor response.
const doctorPingTimeout = 2 * time.Second

// defaultDoctorMasterKeyRotationWarnAge is the age GET
// /api/v1/system/doctor's master_key_rotation check warns beyond,
// overridable via APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS
// (WithDoctorMasterKeyRotationWarnAge). A soft nudge, never fail: this
// platform has no way to know whether skipping a rotation is actually
// unsafe for a given operator.
const defaultDoctorMasterKeyRotationWarnAge = 365 * 24 * time.Hour

type doctorCheckResource struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type systemDoctorResponse struct {
	OK     bool                  `json:"ok"`
	Checks []doctorCheckResource `json:"checks"`
}

// handleSystemDoctor handles GET /api/v1/system/doctor: an operator
// preflight bundle of the individual checks GET /api/v1/system/status
// already surfaces (Docker, disk, data dir) plus a few doctor-only ones
// (data dir writability, ingress ports, SQLite reachability). Always
// 200, same "an unconfigured or failed optional check is a real,
// reportable status, never a 5xx" shape handleSystemStatus already
// follows; OK reflects only whether any individual check is fail.
func (rt *Router) handleSystemDoctor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := []doctorCheckResource{
		rt.doctorCheckDocker(ctx),
		rt.doctorCheckDiskSpace(),
		rt.doctorCheckDataDirWritable(),
		doctorCheckPort(80),
		doctorCheckPort(443),
		rt.doctorCheckDatabase(ctx),
		rt.doctorCheckMasterKeyRotation(ctx),
		doctorCheckFirewallCtx(ctx),
	}

	ok := true
	for _, c := range checks {
		if c.Status == doctorStatusFail {
			ok = false
			break
		}
	}

	writeJSON(w, http.StatusOK, systemDoctorResponse{OK: ok, Checks: checks})
}

func (rt *Router) doctorCheckDocker(ctx context.Context) doctorCheckResource {
	const code, name = "docker", "Docker daemon"
	if rt.dockerPinger == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no Docker reachability check configured"}
	}
	if err := rt.dockerPinger.Ping(ctx); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: err.Error()}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "reachable"}
}

func (rt *Router) doctorCheckDiskSpace() doctorCheckResource {
	const code, name = "disk_space", "Disk space"
	if rt.dataDir == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "data directory not configured"}
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(rt.dataDir, &stat); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read disk usage: %s", err)}
	}
	freeBytes := int64(stat.Bavail) * int64(stat.Bsize) //nolint:gosec // statfs fields are always non-negative in practice

	threshold := rt.doctorDiskWarningBytes
	if threshold <= 0 {
		threshold = defaultDoctorDiskWarningBytes
	}
	if freeBytes < threshold {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("%d bytes free, below the %d byte warning threshold", freeBytes, threshold)}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d bytes free", freeBytes)}
}

func (rt *Router) doctorCheckDataDirWritable() doctorCheckResource {
	const code, name = "data_dir_writable", "Data directory writable"
	if rt.dataDir == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "data directory not configured"}
	}

	f, err := os.CreateTemp(rt.dataDir, ".doctor-write-check-*")
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: err.Error()}
	}
	path := f.Name()
	_ = f.Close()
	if err := os.Remove(path); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("wrote a test file but could not remove it: %s", err)}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "writable"}
}

// doctorCheckPort is a best-effort local bind check for the embedded
// Caddy ingress's two listen ports. A permission error (binding <1024
// without CAP_NET_BIND_SERVICE) can't distinguish "in use" from "not
// allowed to check", so it degrades to unknown rather than fail.
func doctorCheckPort(port int) doctorCheckResource {
	code := fmt.Sprintf("port_%d", port)
	name := fmt.Sprintf("Port %d available", port)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		_ = ln.Close()
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "available"}
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: "already in use"}
	}
	if errors.Is(err, os.ErrPermission) {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "cannot check without elevated privileges"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: err.Error()}
}

func (rt *Router) doctorCheckDatabase(ctx context.Context) doctorCheckResource {
	const code, name = "database", "Control plane database"
	if rt.dbPinger == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no database reachability check configured"}
	}

	pingCtx, cancel := context.WithTimeout(ctx, doctorPingTimeout)
	defer cancel()
	if err := rt.dbPinger.PingContext(pingCtx); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: err.Error()}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "reachable"}
}

// doctorCheckMasterKeyRotation surfaces how long it has been since the
// envelope-encryption master key was last rotated (RotateMasterKey), a
// soft nudge per section 4.10/10 of this project's own design doc:
// never fail, since this platform cannot know whether skipping
// rotation is actually unsafe for a given operator.
func (rt *Router) doctorCheckMasterKeyRotation(ctx context.Context) doctorCheckResource {
	const code, name = "master_key_rotation", "Master key rotation"
	if rt.masterKeyRotator == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no master key configured"}
	}

	rotatedAt, ok, err := rt.masterKeyRotator.GetMasterKeyRotatedAt(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read rotation history: %s", err)}
	}
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "never rotated (informational, not a requirement)"}
	}

	age := time.Since(rotatedAt)
	threshold := rt.doctorMasterKeyRotationWarnAge
	if threshold <= 0 {
		threshold = defaultDoctorMasterKeyRotationWarnAge
	}
	if age > threshold {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("last rotated %s ago, consider rotating again", age.Round(time.Hour))}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("last rotated %s ago", age.Round(time.Hour))}
}
