package authengine

import (
	"os"
	"runtime/debug"
	"strconv"
	"time"
)

const (
	// EnvShadowQueue sizes the shadow comparison queue.
	EnvShadowQueue = "APP_AUTH_ENGINE_SHADOW_QUEUE"
	// EnvShadowWorkers sets how many goroutines compare tokens.
	EnvShadowWorkers = "APP_AUTH_ENGINE_SHADOW_WORKERS"
	// EnvShadowMismatchLog sets how many recent mismatches the status endpoint keeps.
	EnvShadowMismatchLog = "APP_AUTH_ENGINE_SHADOW_MISMATCH_LOG"

	defaultShadowQueue       = 256
	defaultShadowWorkers     = 2
	defaultShadowMismatchLog = 50

	libraryModule = "github.com/glincker/theauth-go/v2"
)

// Mismatch kinds recorded by the shadow comparison.
const (
	MismatchDecision  = "decision"
	MismatchOwner     = "owner"
	MismatchAbilities = "abilities"
)

// ShadowConfig bounds the shadow comparison so it can never slow a request.
type ShadowConfig struct {
	QueueSize   int
	Workers     int
	MismatchLog int
}

// ShadowConfigFromEnv reads the APP_AUTH_ENGINE_SHADOW_* overrides.
func ShadowConfigFromEnv() ShadowConfig {
	return ShadowConfig{
		QueueSize:   envPositive(EnvShadowQueue, defaultShadowQueue),
		Workers:     envPositive(EnvShadowWorkers, defaultShadowWorkers),
		MismatchLog: envPositive(EnvShadowMismatchLog, defaultShadowMismatchLog),
	}
}

func envPositive(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// Mismatch is one disagreement between the legacy engine and the library.
// It carries ids and ability names only, never a secret.
type Mismatch struct {
	At               time.Time `json:"at"`
	Kind             string    `json:"kind"`
	TokenID          string    `json:"token_id,omitempty"`
	LegacyOwnerID    string    `json:"legacy_owner_id,omitempty"`
	LibraryOwnerID   string    `json:"library_owner_id,omitempty"`
	LegacyAccepted   bool      `json:"legacy_accepted"`
	LibraryAccepted  bool      `json:"library_accepted"`
	LegacyAbilities  []string  `json:"legacy_abilities"`
	LibraryAbilities []string  `json:"library_abilities"`
}

// Status is what the auth-engine status endpoint reports.
type Status struct {
	Mode           string     `json:"mode"`
	LibraryVersion string     `json:"library_version"`
	Compared       uint64     `json:"compared"`
	Matched        uint64     `json:"matched"`
	Mismatched     uint64     `json:"mismatched"`
	Dropped        uint64     `json:"dropped"`
	Skipped        uint64     `json:"skipped"`
	Errors         uint64     `json:"errors"`
	Mismatches     []Mismatch `json:"mismatches"`
}

// LibraryVersion reports the linked library version, or "unknown".
func LibraryVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range info.Deps {
		if d.Path == libraryModule {
			if d.Replace != nil && d.Replace.Version != "" {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return "unknown"
}
