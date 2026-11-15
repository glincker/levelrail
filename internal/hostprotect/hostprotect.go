// Package hostprotect writes the OOM protection drop-in for the control plane
// unit. install.sh writes the same file; a test keeps the two from drifting.
package hostprotect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Marker is the first line of the drop-in. A file without it was edited by hand and is never replaced.
const Marker = "# Written by install.sh (levelrail host protection)."

// EnvOptOut set to 0 skips the drop-in, matching install.sh.
const EnvOptOut = "LEVELRAIL_PROTECT"

// Result says what Apply did.
type Result string

// The outcomes of Apply.
const (
	Skipped Result = "skipped"
	Current Result = "current"
	Wrote   Result = "wrote"
	Kept    Result = "kept"
)

// Render returns the drop-in text for a service. Values come from the same
// LEVELRAIL_* variables as install.sh, with the same defaults and checks.
func Render(service string, getenv func(string) string) (string, error) {
	val := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	oom := val("LEVELRAIL_OOM_SCORE_ADJUST", "-900")
	rsec := val("LEVELRAIL_RESTART_SEC", "5")
	burst := val("LEVELRAIL_START_LIMIT_BURST", "20")
	window := val("LEVELRAIL_START_LIMIT_INTERVAL_SEC", "600")
	mlow := val("LEVELRAIL_MEMORY_LOW", "256M")
	if strings.Trim(oom+rsec+burst+window, "0123456789-") != "" {
		return "", errors.New("OOM score, restart seconds, start limit burst and interval must be integers")
	}
	if strings.Trim(mlow, "0123456789KMGkmg") != "" {
		return "", errors.New("memory low must look like 256M")
	}
	dir := "/etc/systemd/system/" + service + ".service.d"
	var b strings.Builder
	fmt.Fprintf(&b, `%s
# Why: on a busy host the kernel OOM killer picks the biggest victims, and a
# killed control plane leaves every app unmanaged. A negative OOMScoreAdjust makes
# other processes die first. Restart=always with a start limit restarts a killed
# process after RestartSec but stops a crash loop instead of spinning forever.
# Override: add %s/40-local.conf (it sorts later), then
# systemctl daemon-reload && systemctl restart %s.
[Unit]
StartLimitIntervalSec=%s
StartLimitBurst=%s

[Service]
OOMScoreAdjust=%s
Restart=always
RestartSec=%s
TasksMax=infinity
MemoryLow=%s
`, Marker, dir, service, window, burst, oom, rsec, mlow)
	return b.String(), nil
}

// Host holds the paths and hooks Apply needs, so tests can point it at a temp tree.
type Host struct {
	Service    string
	UnitDir    string
	SystemdRun string
	Geteuid    func() int
	Getenv     func(string) string
	Reload     func(context.Context) error
}

// DefaultHost targets the real host for one service name.
func DefaultHost(service string, reload func(context.Context) error) Host {
	return Host{Service: service, UnitDir: "/etc/systemd/system", SystemdRun: "/run/systemd/system", Geteuid: os.Geteuid, Getenv: os.Getenv, Reload: reload}
}

// Apply writes the drop-in when running as root under systemd with an installed
// unit. It never restarts the service; the caller's own restart applies it.
func (h Host) Apply(ctx context.Context) (Result, error) {
	if h.Getenv(EnvOptOut) == "0" || h.Geteuid() != 0 {
		return Skipped, nil
	}
	if st, err := os.Stat(h.SystemdRun); err != nil || !st.IsDir() {
		return Skipped, nil
	}
	if _, err := os.Stat(filepath.Join(h.UnitDir, h.Service+".service")); err != nil {
		return Skipped, nil
	}
	text, err := Render(h.Service, h.Getenv)
	if err != nil {
		return Skipped, err
	}
	dir := filepath.Join(h.UnitDir, h.Service+".service.d")
	path := filepath.Join(dir, "30-protection.conf")
	if old, rerr := os.ReadFile(path); rerr == nil { //nolint:gosec // fixed name under the unit directory
		if !bytes.Contains(old, []byte(Marker)) {
			return Kept, nil
		}
		if string(old) == text {
			return Current, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // systemd reads drop-ins as any user
		return Skipped, err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil { //nolint:gosec // systemd unit files are world readable
		return Skipped, err
	}
	if h.Reload != nil {
		if err := h.Reload(ctx); err != nil {
			return Wrote, fmt.Errorf("daemon-reload: %w", err)
		}
	}
	return Wrote, nil
}
