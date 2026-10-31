// Package mailbackup is the pure core of mail service backup and restore:
// mox backup and verifydata via a Runner, a streamed tar, zstd, age archive
// into a Sink, a manifest, and a guarded restore. It has no API or DB code.
package mailbackup

import (
	"context"
	"fmt"
	"io"
	"time"

	"filippo.io/age"
)

// Runner executes `mox <args>` inside the mail container and returns stdout.
// It is an interface so tests never need a container.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// Sink stores named blobs. Put must stream r without buffering it whole.
type Sink interface {
	Put(ctx context.Context, name string, r io.Reader) error
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, name string) error
}

// Pruner is implemented by sinks that enforce a size cap after a backup.
type Pruner interface {
	Prune(ctx context.Context) ([]string, error)
}

// Manifest describes one backup. It is stored next to the archive, because
// it carries the archive checksum and so cannot live inside it.
type Manifest struct {
	ID          string    `json:"id"`
	MoxVersion  string    `json:"mox_version"`
	CreatedAt   time.Time `json:"created_at"`
	Files       int       `json:"files"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
	ArchiveName string    `json:"archive_name"`
	Verified    bool      `json:"verified"`
}

// BackupSpec configures one backup run.
type BackupSpec struct {
	Runner Runner
	Sink   Sink
	// StagingHost and StagingContainer are the same directory seen from
	// the host and from inside the mail container.
	StagingHost      string
	StagingContainer string
	Recipients       []age.Recipient
	Now              func() time.Time
	// Fetch copies the container staging dir to the host one when they are
	// not the same directory (a Docker volume the control plane cannot see).
	Fetch func(ctx context.Context, containerDir, hostDir string) error
	// FetchConfig copies the mox config dir (DKIM keys, certificates) into
	// hostDir; nil leaves the config out of the archive.
	FetchConfig func(ctx context.Context, hostDir string) error
	// Cleanup removes the container staging dir; nil means host removal covers it.
	Cleanup func(ctx context.Context, containerDir string)
	// Progress reports each step by name.
	Progress func(step string)
}

// RestoreSpec configures one restore run.
type RestoreSpec struct {
	Runner     Runner
	Sink       Sink
	ID         string
	DestDir    string
	Identities []age.Identity
}

// RestoreReport says what a restore did, or with dryRun would do.
type RestoreReport struct {
	Manifest Manifest
	Files    int
	Bytes    int64
	DryRun   bool
}

// CapError means a backup cannot fit the local folder cap even alone.
type CapError struct {
	CapBytes  int64
	NeedBytes int64
}

func (e *CapError) Error() string {
	return fmt.Sprintf("mail backup needs %d bytes but the local cap is %d bytes", e.NeedBytes, e.CapBytes)
}

// DowngradeError means the target mox is older than the backup's mox.
type DowngradeError struct{ Target, Backup string }

func (e *DowngradeError) Error() string {
	return fmt.Sprintf("target mox %s is older than the backup's mox %s; restore refused", e.Target, e.Backup)
}

// ChecksumError means the stored archive does not match its manifest.
type ChecksumError struct{ Want, Got string }

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("archive checksum mismatch: manifest %s, archive %s", e.Want, e.Got)
}

func archiveName(id string) string  { return "mail-" + id + ".tar.zst.age" }
func manifestName(id string) string { return "mail-" + id + ".manifest.json" }

// ManifestName is the sink name of a backup's manifest.
func ManifestName(id string) string { return manifestName(id) }
