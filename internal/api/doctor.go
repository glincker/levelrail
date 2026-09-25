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

	"github.com/GLINCKER/levelrail/internal/diskspace"
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

// defaultDoctorHTTPPort/defaultDoctorHTTPSPort are the ports GET
// /api/v1/system/doctor's port checks probe when WithDoctorIngressPorts
// hasn't overridden them: the embedded ingress's own compiled-in
// defaults (internal/reconcile/ingress's defaultHTTPListenAddr/
// defaultListenAddr).
const (
	defaultDoctorHTTPPort  = 80
	defaultDoctorHTTPSPort = 443
)

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
	// Fix is a concrete, copy-pasteable shell command that resolves this
	// check when it isn't ok, or empty when there's no single command
	// that would (a database ping failing needs investigation, not a
	// command). Rendered as a code block by both the CLI and the
	// dashboard.
	Fix string `json:"fix,omitempty"`
	// DocsPath is a path relative to this instance's docs site root
	// (brand.DocsURL), e.g. "/troubleshooting#clock-skew", surfaced as a
	// HelpLink alongside Fix. Empty when Fix is empty.
	DocsPath string `json:"docs_path,omitempty"`
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
	httpPort := rt.doctorHTTPPort
	if httpPort == 0 {
		httpPort = defaultDoctorHTTPPort
	}
	httpsPort := rt.doctorHTTPSPort
	if httpsPort == 0 {
		httpsPort = defaultDoctorHTTPSPort
	}
	checks := []doctorCheckResource{
		rt.doctorCheckDocker(ctx),
		rt.doctorCheckDiskSpace(),
		rt.doctorCheckDataDirWritable(),
		rt.doctorCheckPort(httpPort),
		rt.doctorCheckPort(httpsPort),
		rt.doctorCheckDatabase(ctx),
		rt.doctorCheckMasterKeyRotation(ctx),
		rt.doctorCheckSecretBinding(ctx),
		rt.doctorCheckStaleSecrets(ctx),
		rt.doctorCheckControlPlaneBackup(),
		doctorCheckFirewallCtx(ctx),
		rt.doctorCheckRAM(),
		rt.doctorCheckCPU(),
	}
	checks = append(checks, rt.doctorRunNetworkChecks(ctx, httpPort, httpsPort)...)
	checks = append(checks, rt.doctorCheckGPUs(ctx)...)
	checks = append(checks, rt.doctorCheckGPUPlacement(ctx)...)

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
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("%s free, below the %s warning threshold", diskspace.HumanBytes(freeBytes), diskspace.HumanBytes(threshold))}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%s free", diskspace.HumanBytes(freeBytes))}
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

// IngressPortOwner reports whether this control plane's own embedded
// ingress (internal/ingress.Driver) currently has a listener bound to
// port. *ingress.Driver satisfies this.
type IngressPortOwner interface {
	OwnsPort(port int) bool
}

// doctorCheckPort is a best-effort local bind check for the embedded
// Caddy ingress's listen ports. A bind failure here is not automatically
// a problem: this handler runs inside the same process as this control
// plane's own ingress, which is normally already bound to these ports by
// the time an operator runs doctor against a live instance.
// rt.ingressPortOwner (nil until wired via WithIngressPortOwner) tells
// that expected case apart from a port genuinely held by something else,
// which still reports fail. A permission error (binding <1024 without
// CAP_NET_BIND_SERVICE) can't distinguish "in use" from "not allowed to
// check", so it degrades to unknown rather than fail.
func (rt *Router) doctorCheckPort(port int) doctorCheckResource {
	code := fmt.Sprintf("port_%d", port)
	name := fmt.Sprintf("Port %d available", port)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		_ = ln.Close()
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "available"}
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		if rt.ingressPortOwner != nil && rt.ingressPortOwner.OwnsPort(port) {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "in use by this control plane's own ingress, as expected"}
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: "in use by another process"}
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

// doctorControlPlaneBackupWarnAge is how old the newest control plane
// snapshot may get before the doctor warns.
const doctorControlPlaneBackupWarnAge = 3 * 24 * time.Hour

// doctorCheckControlPlaneBackup warns when the newest control plane
// snapshot is stale. Never fails: a missing backup is advice, not an outage.
func (rt *Router) doctorCheckControlPlaneBackup() doctorCheckResource {
	const code, name = "control_plane_backup", "Control plane backup"
	if rt.cpBackups == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "control plane backups are not configured"}
	}
	if rt.cpBackupScheduleOff {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "scheduled backups are disabled (APP_CONTROL_PLANE_BACKUP_INTERVAL=0)"}
	}
	list, err := rt.cpBackups.List()
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list backups: %s", err)}
	}
	if len(list) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no snapshot yet, the first scheduled one is taken after the backup interval"}
	}
	newest := list[0]
	age := time.Since(newest.CreatedAt)
	if age > doctorControlPlaneBackupWarnAge {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("newest snapshot is %d days old", int(age/(24*time.Hour))),
			Fix:      "levelrail-cli control-plane-backups create",
			DocsPath: "/control-plane-backup#automatic-snapshots",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("newest snapshot is %s old", age.Round(time.Minute))}
}

// doctorCheckStaleSecrets surfaces how many secret-backed values (per-app
// and shared-env alike) haven't been set or rotated within
// effectiveSecretRotationWarnAge, a distinct signal from
// doctorCheckMasterKeyRotation above: that one is about the envelope-
// encryption master key itself, this one is about the individual secret
// values it encrypts. A soft nudge, never fail, the same reasoning
// doctorCheckMasterKeyRotation's own doc comment gives: this platform
// cannot know whether an old secret is actually unsafe for a given
// operator.
func (rt *Router) doctorCheckStaleSecrets(ctx context.Context) doctorCheckResource {
	const code, name = "stale_secrets", "Secret rotation"
	if rt.staleSecretCounter == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no master key configured"}
	}

	threshold := rt.effectiveSecretRotationWarnAge()
	cutoff := time.Now().UTC().Add(-threshold)
	n, err := rt.staleSecretCounter.CountStaleSecrets(ctx, cutoff)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not count stale secrets: %s", err)}
	}
	thresholdDays := int(threshold / (24 * time.Hour))
	if n > 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("%d secret(s) not rotated in over %d days, consider rotating them", n, thresholdDays)}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("no secrets older than %d days", thresholdDays)}
}
