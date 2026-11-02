package main

import (
	"os"
	"path/filepath"
)

// ensureConfigHome points XDG_CONFIG_HOME under the data dir when a service
// manager gives the process neither it nor HOME, so libraries that look up a
// user config dir do not warn on every start.
func ensureConfigHome(getenv func(string) string, setenv func(string, string) error, dataDir string) {
	if getenv("XDG_CONFIG_HOME") != "" || getenv("HOME") != "" {
		return
	}
	_ = setenv("XDG_CONFIG_HOME", filepath.Join(dataDir, ".config"))
}

func init() {
	ensureConfigHome(os.Getenv, os.Setenv, dataDirFromEnv())
}
