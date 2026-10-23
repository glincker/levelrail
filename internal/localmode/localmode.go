// Package localmode holds the pieces shared by the CLI's dev verb and the control plane's local-instance behavior.
package localmode

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// EnvEnabled turns a control plane into a loopback-only local instance.
	EnvEnabled = "APP_LOCAL_MODE"
	// EnvSupervised tells the control plane a parent will relaunch it after a reset exit.
	EnvSupervised = "APP_RESTART_SUPERVISED"

	resetMarkerName = ".reset-requested"
	adminFileName   = "local-admin.json"
	stateFileName   = "local-state.json"
)

// Enabled reports whether getenv says this process is a local instance.
func Enabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(EnvEnabled))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ResetMarkerPath is the file whose presence asks the supervisor to wipe the data dir on the next relaunch.
func ResetMarkerPath(dataDir string) string { return filepath.Join(dataDir, resetMarkerName) }

// AdminFilePath is where the generated admin credential is kept, mode 0600.
func AdminFilePath(dataDir string) string { return filepath.Join(dataDir, adminFileName) }

// StateFilePath is where the running supervisor records its pid and URLs.
func StateFilePath(dataDir string) string { return filepath.Join(dataDir, stateFileName) }

// RequestReset drops the reset marker.
func RequestReset(dataDir string) error {
	if err := os.WriteFile(ResetMarkerPath(dataDir), []byte("reset\n"), 0o600); err != nil {
		return fmt.Errorf("localmode: write reset marker: %w", err)
	}
	return nil
}

// ResetRequested reports whether a reset marker is present.
func ResetRequested(dataDir string) bool {
	_, err := os.Stat(ResetMarkerPath(dataDir))
	return err == nil
}

// Wipe removes everything inside dataDir but keeps the directory itself; it refuses a non-empty directory a local instance did not create.
func Wipe(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" || dataDir == filepath.Dir(dataDir) {
		return fmt.Errorf("localmode: refusing to wipe %q", dataDir)
	}
	entries, err := os.ReadDir(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("localmode: read data dir: %w", err)
	}
	if _, err := os.Stat(AdminFilePath(dataDir)); err != nil && len(entries) > 0 {
		return fmt.Errorf("localmode: %q was not created by a local instance, refusing to wipe it", dataDir)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dataDir, e.Name())); err != nil {
			return fmt.Errorf("localmode: remove %s: %w", e.Name(), err)
		}
	}
	return nil
}

// ValidateListenAddr rejects a dashboard listen address that is reachable beyond this machine.
func ValidateListenAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("localmode: listen address %q: %w", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("localmode: listen address %q is not loopback; a local instance only binds 127.0.0.1", addr)
}

// DashboardURL is the dashboard address for a loopback port.
func DashboardURL(port int) string { return "http://localhost:" + strconv.Itoa(port) }

// AppURL is the address of an app served on host by an HTTP ingress listening on httpPort.
func AppURL(host string, httpPort int) string {
	if httpPort == 80 {
		return "http://" + host
	}
	return "http://" + host + ":" + strconv.Itoa(httpPort)
}
