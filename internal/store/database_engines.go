package store

import (
	"embed"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed database_engines.yaml
var databaseEnginesFS embed.FS

// DatabaseEngineInfo is one row of database_engines.yaml: identity and
// display metadata for a managed database engine this control plane can
// create. See that file's own doc comment for what does and doesn't
// belong here (display data only, not the real per-engine reconcile
// behavior, which stays Go code).
type DatabaseEngineInfo struct {
	ID             string `yaml:"id"`
	Label          string `yaml:"label"`
	DefaultVersion string `yaml:"default_version"`
}

type databaseEnginesFile struct {
	Engines []DatabaseEngineInfo `yaml:"engines"`
}

var (
	databaseEngines     []DatabaseEngineInfo
	databaseEnginesOnce sync.Once
	databaseEnginesErr  error
)

// SupportedDatabaseEngines returns every database engine this control
// plane can create, parsed from the embedded registry once and cached:
// the registry never changes at runtime, so there's no reason to
// re-parse it per call, the same reasoning internal/spec's own
// compiledAppSchema already applies to its embedded JSON Schema.
func SupportedDatabaseEngines() ([]DatabaseEngineInfo, error) {
	databaseEnginesOnce.Do(func() {
		raw, err := databaseEnginesFS.ReadFile("database_engines.yaml")
		if err != nil {
			databaseEnginesErr = fmt.Errorf("store: read embedded database engine registry: %w", err)
			return
		}
		var parsed databaseEnginesFile
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			databaseEnginesErr = fmt.Errorf("store: parse embedded database engine registry: %w", err)
			return
		}
		databaseEngines = parsed.Engines
	})
	return databaseEngines, databaseEnginesErr
}

// IsSupportedEngine reports whether id is a real, registered database
// engine. internal/api's validateDatabaseResource and
// internal/store.SaveDesiredDatabase's own callers use this instead of
// each hardcoding its own postgres/redis/mysql comparison chain, so
// adding a new engine's identity here is one file to edit, not a grep
// across the codebase for every hardcoded chain.
func IsSupportedEngine(id string) (bool, error) {
	engines, err := SupportedDatabaseEngines()
	if err != nil {
		return false, err
	}
	for _, e := range engines {
		if e.ID == id {
			return true, nil
		}
	}
	return false, nil
}
