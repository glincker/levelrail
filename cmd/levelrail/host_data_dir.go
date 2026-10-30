package main

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	installedDataDir = "/var/lib/levelrail-data"
	systemdUnitDir   = "/etc/systemd/system"
	systemdUnitName  = "levelrail.service"
)

var hostCommands = map[string]bool{
	"setup-token": true, "recover-admin": true, "approve-signin": true, "emergency-token": true,
	"access-policy-reset": true, "restore-db": true, "wrap-master-key": true, "restore-snapshot": true, "restore": true,
}

// adoptInstalledDataDir lets a host subcommand run from a login shell on an
// installed host: it reads the service's own APP_DATA_DIR, then the
// installer default, and leaves the dev default when neither exists.
func adoptInstalledDataDir(unitDir string) string {
	if os.Getenv("APP_DATA_DIR") != "" {
		return ""
	}
	dir := unitDataDir(unitDir)
	if dir == "" {
		if _, err := os.Stat(filepath.Join(installedDataDir, storeFilename)); err == nil {
			dir = installedDataDir
		}
	}
	if dir != "" {
		_ = os.Setenv("APP_DATA_DIR", dir)
	}
	return dir
}

// unitDataDir returns APP_DATA_DIR from the unit file or its drop-ins.
func unitDataDir(unitDir string) string {
	files, _ := filepath.Glob(filepath.Join(unitDir, systemdUnitName+".d", "*.conf"))
	files = append(files, filepath.Join(unitDir, systemdUnitName))
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // fixed systemd unit paths
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Environment=") {
				continue
			}
			for _, kv := range strings.Fields(strings.Trim(strings.TrimPrefix(line, "Environment="), `"`)) {
				if v, ok := strings.CutPrefix(strings.Trim(kv, `"`), "APP_DATA_DIR="); ok && v != "" {
					return v
				}
			}
		}
	}
	return ""
}
