// Package upgrade runs the read-only checks that gate a platform self-upgrade.
package upgrade

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Check statuses.
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusFail    = "fail"
	StatusUnknown = "unknown"
)

// Check is one preflight result.
type Check struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Asset names the release workflow publishes next to the binaries.
const (
	ChecksumsAsset  = "checksums.txt"
	SignatureAsset  = "checksums.txt.sigstore.json"
	envKnownBadFile = "APP_DOCKER_KNOWN_BAD_FILE"
	envMinFreeBytes = "APP_UPGRADE_MIN_FREE_BYTES"
	envMaxBackupAge = "APP_UPGRADE_MAX_BACKUP_AGE"
	defaultMinFree  = int64(2 << 30)
	defaultBackupAg = 26 * time.Hour
)

//go:embed docker_known_bad.json
var embeddedKnownBad []byte

// KnownBad is one Docker Engine version prefix that must not run an upgrade.
type KnownBad struct {
	Prefix string `json:"prefix"`
	Reason string `json:"reason"`
}

type knownBadFile struct {
	Entries []KnownBad `json:"entries"`
}

// LoadKnownBad reads the known-bad Docker list from APP_DOCKER_KNOWN_BAD_FILE
// when set, otherwise from the embedded data file.
func LoadKnownBad(lookup func(string) (string, bool)) ([]KnownBad, error) {
	raw := embeddedKnownBad
	if path, ok := lookup(envKnownBadFile); ok && path != "" {
		b, err := os.ReadFile(path) //nolint:gosec // operator-supplied override path
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		raw = b
	}
	var f knownBadFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse docker known-bad list: %w", err)
	}
	return f.Entries, nil
}

// Inputs is everything Run needs, injected so tests use fakes.
type Inputs struct {
	Lookup        func(string) (string, bool)
	DockerVersion func(ctx context.Context) (string, error)
	FreeBytes     func() (int64, error)
	NewestBackup  func() (time.Time, bool, error)
	Now           time.Time
	AssetNames    []string
	ReleaseKnown  bool
}

// Run evaluates every preflight check. It never changes anything.
func Run(ctx context.Context, in Inputs) []Check {
	return []Check{
		releaseAssetsCheck(in),
		dockerCheck(ctx, in),
		diskCheck(in),
		backupCheck(in),
	}
}

// Blocked reports whether any check is a hard failure.
func Blocked(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

func releaseAssetsCheck(in Inputs) Check {
	c := Check{Code: "release_signature", Name: "Release checksums and signature"}
	if !in.ReleaseKnown {
		c.Status, c.Message = StatusUnknown, "latest release could not be fetched"
		return c
	}
	have := map[string]bool{}
	for _, n := range in.AssetNames {
		have[n] = true
	}
	var missing []string
	for _, want := range []string{ChecksumsAsset, SignatureAsset} {
		if !have[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		c.Status, c.Message = StatusFail, "release is missing "+strings.Join(missing, ", ")
		return c
	}
	c.Status, c.Message = StatusOK, "checksums.txt and its signature are published; the installer verifies both"
	return c
}

func dockerCheck(ctx context.Context, in Inputs) Check {
	c := Check{Code: "docker_engine", Name: "Docker Engine version"}
	if in.DockerVersion == nil {
		c.Status, c.Message = StatusUnknown, "no Docker connection configured"
		return c
	}
	ver, err := in.DockerVersion(ctx)
	if err != nil {
		c.Status, c.Message = StatusFail, "could not read the Docker Engine version: "+err.Error()
		return c
	}
	bad, err := LoadKnownBad(in.Lookup)
	if err != nil {
		c.Status, c.Message = StatusWarn, "engine "+ver+", known-bad list unreadable: "+err.Error()
		return c
	}
	for _, b := range bad {
		if b.Prefix != "" && strings.HasPrefix(ver, b.Prefix) {
			c.Status, c.Message = StatusFail, fmt.Sprintf("engine %s is on the known-bad list: %s", ver, b.Reason)
			return c
		}
	}
	c.Status, c.Message = StatusOK, "engine "+ver+" is not on the known-bad list"
	return c
}

func diskCheck(in Inputs) Check {
	c := Check{Code: "disk_space", Name: "Free disk space"}
	if in.FreeBytes == nil {
		c.Status, c.Message = StatusUnknown, "data directory not configured"
		return c
	}
	free, err := in.FreeBytes()
	if err != nil {
		c.Status, c.Message = StatusUnknown, "could not read disk usage: "+err.Error()
		return c
	}
	floor := envInt64(in.Lookup, envMinFreeBytes, defaultMinFree)
	if free < floor {
		c.Status, c.Message = StatusFail, fmt.Sprintf("%d MiB free, upgrade needs at least %d MiB", free>>20, floor>>20)
		return c
	}
	c.Status, c.Message = StatusOK, fmt.Sprintf("%d MiB free", free>>20)
	return c
}

func backupCheck(in Inputs) Check {
	c := Check{Code: "control_plane_backup", Name: "Recent control plane backup"}
	if in.NewestBackup == nil {
		c.Status, c.Message = StatusUnknown, "backups are not configured"
		return c
	}
	newest, ok, err := in.NewestBackup()
	if err != nil {
		c.Status, c.Message = StatusUnknown, "could not list backups: "+err.Error()
		return c
	}
	if !ok {
		c.Status, c.Message = StatusWarn, "no backup exists yet; one is taken automatically before migrations run"
		return c
	}
	maxAge := defaultBackupAg
	if d, err := time.ParseDuration(lookupOr(in.Lookup, envMaxBackupAge, "")); err == nil && d > 0 {
		maxAge = d
	}
	if age := in.Now.Sub(newest); age > maxAge {
		c.Status, c.Message = StatusWarn, fmt.Sprintf("newest backup is %s old", age.Round(time.Minute))
		return c
	}
	c.Status, c.Message = StatusOK, "newest backup is "+in.Now.Sub(newest).Round(time.Minute).String()+" old"
	return c
}

func lookupOr(lookup func(string) (string, bool), key, def string) string {
	if lookup == nil {
		return def
	}
	if v, ok := lookup(key); ok && v != "" {
		return v
	}
	return def
}

func envInt64(lookup func(string) (string, bool), key string, def int64) int64 {
	var n int64
	if _, err := fmt.Sscanf(lookupOr(lookup, key, ""), "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}
