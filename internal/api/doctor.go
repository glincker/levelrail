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

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/kit/diskspace"
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

// Defaults match the embedded ingress's compiled-in listen addresses.
const (
	defaultDoctorHTTPPort  = 80
	defaultDoctorHTTPSPort = 443
)

// doctorPortCheckTimeout bounds the SQLite ping doctor.go issues: a
// stuck check must never hang the whole doctor response.
const doctorPingTimeout = 2 * time.Second

// A soft warning, never a failure, since skipping rotation may be safe for some operators.
const defaultDoctorMasterKeyRotationWarnAge = 365 * 24 * time.Hour

type doctorCheckResource struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// Fix is empty when no single command resolves the check, such as a database ping failure.
	Fix string `json:"fix,omitempty"`
	// DocsPath is a path relative to this instance's docs site root
	// (brand.DocsURL), e.g. "/troubleshooting#clock-skew", surfaced as a
	// HelpLink alongside Fix. Empty when Fix is empty.
	DocsPath string `json:"docs_path,omitempty"`
	// Class is "incident" or "recommendation" for a check that is not ok,
	// from attention.DoctorClass.
	Class string `json:"class,omitempty"`
	// CheckID is the registered check that produced this result, the id to re-run it by.
	CheckID string `json:"check_id,omitempty"`
	// Group is the section the check belongs to, stamped by the engine.
	Group string `json:"group,omitempty"`
	// Evidence is the raw fact behind Message (a path, a holder, a count).
	Evidence string `json:"evidence,omitempty"`
	// FixID and FixKind name the typed fixit fix for a failing check, when one exists.
	FixID   string `json:"fix_id,omitempty"`
	FixKind string `json:"fix_kind,omitempty"`
}

type systemDoctorResponse struct {
	OK     bool                  `json:"ok"`
	Checks []doctorCheckResource `json:"checks"`
	// Silenced counts failing or warning checks an operator muted; they stay in Checks.
	Silenced int `json:"silenced,omitempty"`
}

// Always 200: a failed check is reported in the body, not as a 5xx.
func (rt *Router) handleSystemDoctor(w http.ResponseWriter, r *http.Request) {
	resp, err := rt.doctorReport(r.Context())
	if err != nil {
		rt.internalError(w, "api: system doctor", err)
		return
	}
	now := time.Now()
	resp.Silenced = silencedDoctorCount(resp.Checks, rt.activeSilences(r.Context(), now), now)
	writeJSON(w, http.StatusOK, resp)
}

// doctorReport runs the checks once per dashboard cache ttl. The report
// does not depend on the caller, so one entry serves every viewer.
func (rt *Router) doctorReport(ctx context.Context) (systemDoctorResponse, error) {
	if rt.demo != nil {
		return demoDoctorReport(), nil
	}
	defer timePhase(ctx, "doctor_checks")()
	if rt.dash.ttl <= 0 {
		return rt.runDoctorChecks(ctx), nil
	}
	v, err := rt.dash.get(ctx, "doctor", func(ctx context.Context) (any, error) {
		runCtx, cancel := context.WithTimeout(ctx, doctorRunTimeout)
		defer cancel()
		return rt.runDoctorChecks(runCtx), nil
	})
	if err != nil {
		return systemDoctorResponse{}, err
	}
	resp, _ := v.(systemDoctorResponse)
	return resp, nil
}

// doctorRunTimeout bounds a cached doctor run, detached from any one request.
const doctorRunTimeout = 30 * time.Second

func (rt *Router) doctorCheckDocker(ctx context.Context) doctorCheckResource {
	const code, name = "docker", "Docker daemon"
	if rt.dockerPinger == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no Docker reachability check configured"}
	}
	if err := rt.pingDocker(ctx); err != nil {
		if errors.Is(err, docker.ErrDaemonUnresponsive) {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: dockerNotRespondingMessage}
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusFail, Message: err.Error()}
	}
	msg := "reachable"
	if v, ok := rt.dockerPinger.(engineVersioner); ok {
		if ver, err := v.ServerVersion(ctx); err == nil && ver != "" {
			msg = "reachable, engine " + ver
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
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

// A bind failure is expected when the in-process ingress already holds the port, so it is not a failure then.
// Permission errors degrade to unknown, since they cannot tell in-use apart from not-allowed.
func (rt *Router) doctorCheckPort(port int) doctorCheckResource {
	code := fmt.Sprintf("port_%d", port)
	name := fmt.Sprintf("Port %d available", port)

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port))
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

// A soft warning, never a failure, since this platform cannot know whether skipping rotation is unsafe.
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
			Fix:      rt.cliName() + " control-plane-backups create",
			DocsPath: "/control-plane-backup#automatic-snapshots",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("newest snapshot is %s old", age.Round(time.Minute))}
}

// A soft warning like the master key check, but about individual secret values rather than the master key.
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

// cliName is the operator-facing CLI command, derived from the brand so a
// rename does not leave stale hints in doctor output.
func (rt *Router) cliName() string {
	if rt.brand == nil || rt.brand.BinaryName == "" {
		return "cli"
	}
	return rt.brand.BinaryName + "-cli"
}
