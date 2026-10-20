// Package backup drives a database backup end to end: dump a managed
// database container's data, stream it straight to an S3-compatible
// bucket, and record the attempt in store.BackupHistory. It composes two
// independent capabilities, each with its own file and its own real
// implementation:
//
//   - Dumper (dump.go): produces a dump stream from a running database
//     container.
//   - Uploader (uploader.go): writes a stream to a configured
//     destination bucket.
//   - Deleter (deleter.go): removes a single object from a configured
//     destination bucket, used by Scheduler's retention pruning.
//
// Runner (runner.go) is the only piece that talks to store and
// internal/secrets; Dumper, Uploader, and Deleter implementations know
// nothing about either, so each is independently testable with a fake
// on the other side of its interface.
package backup

import (
	"context"
	"io"
)

// Destination is everything an Uploader needs to reach a specific
// bucket, credentials included. Built fresh per call from a
// store.BackupTarget plus its resolved internal/secrets values (see
// store.BackupTargetSecretsKey): never persisted or logged as a whole,
// since AccessKeyID and SecretAccessKey are live credentials.
type Destination struct {
	// Provider is one of store.BackupProviderAWS / R2 / Custom. An
	// Uploader implementation may use it to pick defaults (e.g. AWS's
	// per-region default endpoint), but Endpoint below is always the
	// final authority when non-empty.
	Provider string
	// Endpoint is the S3-compatible API base URL. Empty for
	// BackupProviderAWS (the SDK's own region-based resolver applies);
	// required for R2 and Custom.
	Endpoint string
	Region   string
	Bucket   string

	AccessKeyID     string
	SecretAccessKey string
}

// Uploader writes a single object to a Destination. Implementations
// must read r to completion (or return an error partway through, in
// which case the object must not be left partially visible under key,
// an S3-compatible multipart upload aborted rather than completed
// covers this for free) rather than assume a specific size up front:
// size is a hint for implementations that benefit from knowing it
// (choosing single-PUT vs. multipart), not a hard contract r will
// produce exactly that many bytes.
type Uploader interface {
	Upload(ctx context.Context, dest Destination, key string, r io.Reader, size int64) error
}

// Deleter removes a single object from a Destination, the inverse of
// Uploader. Implementations must treat deleting a key that is already
// gone (or never existed) as success, not an error: S3-compatible
// DeleteObject calls are naturally idempotent this way.
type Deleter interface {
	Delete(ctx context.Context, dest Destination, key string) error
}

// Dumper produces a database dump as a stream, for a single managed
// database container already running under containerName (the same
// name application.ContainerName / the database controller's own
// container-naming convention already produces; Dumper does not
// construct or look this up itself, Runner does).
//
// The returned ReadCloser's Close must be called by every caller once
// done, success or failure, the same contract io.ReadCloser always
// carries; a Dumper backed by a subprocess or a docker exec stream
// depends on Close to release that underlying resource.
type Dumper interface {
	Dump(ctx context.Context, engine, containerName string) (io.ReadCloser, error)
}
