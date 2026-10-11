// Package upgrade fetches the latest release for stable, beta and edge channels from a
// GitHub repo, compares versions, and runs the read-only checks that gate a self-upgrade.
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
	// StatusPending is a retryable block: the condition is expected to clear
	// on its own (a release still being published), so it reads as waiting,
	// not as a failure.
	StatusPending = "pending"
	// StatusSkipped marks a check that does not apply to this target. It
	// never blocks.
	StatusSkipped = "skipped"
)

// Fix actions a check can carry. The dashboard renders each as a button, the
// CLI prints the matching command.
const (
	FixBackupNow = "backup_now"
	FixRecheck   = "recheck"
	FixCommand   = "command"
)

// Check kinds that refine a code for callers that act on them.
const (
	KindReleaseIncomplete = "release_incomplete"
	KindReleaseUnsigned   = "release_unsigned"
	KindBackupStale       = "backup_stale"
	KindBackupMissing     = "backup_missing"
)

// Fix is the one action that can clear a check.
type Fix struct {
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
}

// Check is one preflight result.
type Check struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// Kind refines Code (for example release_incomplete versus release_unsigned).
	Kind string `json:"kind,omitempty"`
	// Retryable is set when waiting and re-checking is the right response.
	Retryable bool `json:"retryable,omitempty"`
	// Protects says in one line what this check keeps safe.
	Protects string `json:"protects,omitempty"`
	// Hint says in plain language what to do when the check is not ok.
	Hint string `json:"hint,omitempty"`
	Fix  *Fix   `json:"fix,omitempty"`
}

// Asset names the release workflow publishes next to the binaries.
const (
	ChecksumsAsset   = "checksums.txt"
	SignatureAsset   = "checksums.txt.sigstore.json"
	envKnownBadFile  = "APP_DOCKER_KNOWN_BAD_FILE"
	envMinFreeBytes  = "APP_UPGRADE_MIN_FREE_BYTES"
	envMaxBackupAge  = "APP_UPGRADE_MAX_BACKUP_AGE"
	envReleaseSettle = "APP_UPGRADE_RELEASE_SETTLE"
	defaultMinFree   = int64(2 << 30)
	defaultBackupAg  = 26 * time.Hour
	defaultSettle    = time.Hour
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
	// ReleasePublishedAt is when the release went public, zero if unknown.
	// It separates a release still being published from a broken one.
	ReleasePublishedAt time.Time
	// ReleaseKind is Release.Kind of the target; an edge image has no
	// binaries, so the checksum and signature checks do not apply to it.
	ReleaseKind string
	// LookPath finds the signature verifier (cosign) on the host; nil reports unknown.
	LookPath func(string) (string, error)
}

// Run evaluates every preflight check. It never changes anything.
func Run(ctx context.Context, in Inputs) []Check {
	return []Check{
		releaseAssetsCheck(in),
		verifierCheck(in),
		dockerCheck(ctx, in),
		diskCheck(in),
		backupCheck(in),
	}
}

// Blocked reports whether any check stops an upgrade: a failure, or a
// retryable pending condition.
func Blocked(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail || c.Status == StatusPending {
			return true
		}
	}
	return false
}

func releaseAssetsCheck(in Inputs) Check {
	c := Check{
		Code: "release_signature", Name: "Release checksums and signature",
		Protects: "Makes sure the binary you install is the one the project published and has not been tampered with.",
	}
	if in.ReleaseKind == KindEdgeImage {
		c.Status, c.Message = StatusSkipped, EdgeNoSignatureNote
		return c
	}
	if !in.ReleaseKnown {
		c.Status, c.Message = StatusUnknown, "latest release could not be fetched"
		c.Hint = "Check that this server can reach github.com, then re-check."
		c.Fix = &Fix{Action: FixRecheck}
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
	if len(missing) == 0 {
		c.Status, c.Message = StatusOK, "checksums.txt and its signature are published"
		return c
	}
	settle := defaultSettle
	if d, err := time.ParseDuration(lookupOr(in.Lookup, envReleaseSettle, "")); err == nil && d > 0 {
		settle = d
	}
	if !in.ReleasePublishedAt.IsZero() && in.Now.Sub(in.ReleasePublishedAt) < settle {
		c.Status, c.Kind, c.Retryable = StatusPending, KindReleaseIncomplete, true
		c.Message = "this release is still being published (" + strings.Join(missing, ", ") + " not uploaded yet, release is " +
			HumanDuration(in.Now.Sub(in.ReleasePublishedAt)) + " old)"
		c.Hint = "Nothing is wrong with your server. Publishing usually takes a few minutes: re-check shortly."
		c.Fix = &Fix{Action: FixRecheck}
		return c
	}
	c.Status, c.Kind = StatusFail, KindReleaseUnsigned
	c.Message = "release is missing " + strings.Join(missing, ", ")
	c.Hint = "This release was published without its checksums or signature, so it cannot be verified and will not be installed. Wait for a fixed release."
	c.Fix = &Fix{Action: FixRecheck}
	return c
}

// VerifierTool is the program the installer uses to check the release signature.
const VerifierTool = "cosign"

// EdgeNoSignatureNote explains why release checks are skipped for edge.
const EdgeNoSignatureNote = "Edge builds are container images, not signed binaries, so there are no checksums to verify."

func verifierCheck(in Inputs) Check {
	c := Check{Code: "release_verifier", Name: "Signature verification tool", Protects: "Lets the upgrade verify the release signature in addition to its checksum."}
	if in.ReleaseKind == KindEdgeImage {
		c.Status, c.Message = StatusSkipped, EdgeNoSignatureNote
		return c
	}
	if in.LookPath == nil {
		c.Status, c.Message = StatusUnknown, "could not look for "+VerifierTool
		return c
	}
	if _, err := in.LookPath(VerifierTool); err != nil {
		c.Status = StatusWarn
		c.Message = VerifierTool + " is not installed on this host, so an upgrade can only check the SHA-256 checksum, not the release signature. Install " + VerifierTool + " to verify both."
		c.Protects = "Lets the upgrade verify the release signature in addition to its checksum."
		c.Hint = "Optional: install " + VerifierTool + " on the host, then re-check."
		c.Fix = &Fix{Action: FixCommand}
		return c
	}
	c.Status, c.Message = StatusOK, VerifierTool+" is installed: the upgrade command below refuses a release whose signature does not verify"
	return c
}

func dockerCheck(ctx context.Context, in Inputs) Check {
	c := Check{
		Code: "docker_engine", Name: "Docker Engine version",
		Protects: "Keeps the upgrade from restarting your apps on a Docker Engine release with a known bug.",
	}
	if in.DockerVersion == nil {
		c.Status, c.Message = StatusUnknown, "no Docker connection configured"
		return c
	}
	ver, err := in.DockerVersion(ctx)
	if err != nil {
		c.Status, c.Message = StatusFail, "could not read the Docker Engine version: "+err.Error()
		c.Hint = "Start Docker on this host (systemctl start docker), then re-check."
		c.Fix = &Fix{Action: FixRecheck}
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
			c.Hint = "Update Docker Engine on the host to a release that is not on the known-bad list, then re-check."
			c.Fix = &Fix{Action: FixRecheck}
			return c
		}
	}
	c.Status, c.Message = StatusOK, "engine "+ver+" is not on the known-bad list"
	return c
}

func diskCheck(in Inputs) Check {
	c := Check{
		Code: "disk_space", Name: "Free disk space",
		Protects: "Makes sure there is room for the new binary and a database snapshot, so the upgrade cannot fill the disk halfway.",
	}
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
		c.Hint = "Free disk space (old images, logs, backups), then re-check."
		c.Fix = &Fix{Action: FixRecheck}
		return c
	}
	c.Status, c.Message = StatusOK, fmt.Sprintf("%d MiB free", free>>20)
	return c
}

func backupCheck(in Inputs) Check {
	c := Check{
		Code: "control_plane_backup", Name: "Recent control plane backup",
		Protects: "Gives the upgrade a restore point, so a failed migration can be undone with no data loss.",
	}
	if in.NewestBackup == nil {
		c.Status, c.Message = StatusUnknown, "backups are not configured"
		return c
	}
	newest, ok, err := in.NewestBackup()
	if err != nil {
		c.Status, c.Message = StatusUnknown, "could not list backups: "+err.Error()
		return c
	}
	maxAge := BackupMaxAge(in.Lookup)
	if !ok {
		c.Status, c.Kind = StatusFail, KindBackupMissing
		c.Message = "no control plane backup exists yet; upgrades need one newer than " + HumanDuration(maxAge) + " (" + envMaxBackupAge + ")"
		c.Hint = "Take a backup now. It takes a few seconds."
		c.Fix = &Fix{Action: FixBackupNow}
		return c
	}
	age := in.Now.Sub(newest)
	if age > maxAge {
		c.Status, c.Kind = StatusFail, KindBackupStale
		c.Message = "newest backup is " + HumanDuration(age) + " old; upgrades need one newer than " + HumanDuration(maxAge) + " (" + envMaxBackupAge + ")"
		c.Hint = "Take a fresh backup now. It takes a few seconds."
		c.Fix = &Fix{Action: FixBackupNow}
		return c
	}
	c.Status, c.Message = StatusOK, "newest backup is "+HumanDuration(age)+" old"
	return c
}

// BackupMaxAge is how old the newest control plane backup may be before an
// upgrade is blocked, from APP_UPGRADE_MAX_BACKUP_AGE, default 26h.
func BackupMaxAge(lookup func(string) (string, bool)) time.Duration {
	if d, err := time.ParseDuration(lookupOr(lookup, envMaxBackupAge, "")); err == nil && d > 0 {
		return d
	}
	return defaultBackupAg
}

// HumanDuration renders d as "11h 18m" or "2d 3h", at minute precision.
func HumanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Minute)
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h, m := int(d.Hours()), int(d.Minutes())%60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		days, h := int(d.Hours())/24, int(d.Hours())%24
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, h)
	}
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
