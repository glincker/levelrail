package selfupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const manifestMaxBytes = 64 << 10

// ErrManifestInvalid means the release manifest parsed but does not describe
// the release that was asked for.
var ErrManifestInvalid = errors.New("release manifest is not valid")

// SchemaResult is what a verified release manifest says about the release.
type SchemaResult struct {
	Version string
	Schema  int
	// Detail is the signature verification note.
	Detail string
}

// FetchReleaseSchema downloads the release manifest of tag with checksums.txt
// and its signature, verifies the manifest against the signed checksums, and
// returns the highest migration version the release supports. It never runs
// the release binary. Size, scheme and redirect limits come from f.
func FetchReleaseSchema(ctx context.Context, f Fetcher, s Signer, tag, parent string) (SchemaResult, error) {
	dir, err := os.MkdirTemp(parent, "schema-")
	if err != nil {
		return SchemaResult{}, fmt.Errorf("create schema scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	f.MaxBytes = manifestMaxBytes
	fetched, err := f.Fetch(ctx, tag, upgrade.ManifestAsset, dir)
	if err != nil {
		return SchemaResult{}, err
	}
	if err := VerifyChecksum(fetched.BinaryPath, fetched.ChecksumsPath, upgrade.ManifestAsset); err != nil {
		return SchemaResult{}, err
	}
	detail, err := s.Verify(ctx, fetched.ChecksumsPath, fetched.BundlePath)
	if err != nil {
		return SchemaResult{}, err
	}
	raw, err := os.ReadFile(fetched.BinaryPath) //nolint:gosec // file inside the scratch directory created above
	if err != nil {
		return SchemaResult{}, fmt.Errorf("read release manifest: %w", err)
	}
	var m upgrade.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return SchemaResult{}, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if m.Version != tag || m.SchemaVersion <= 0 {
		return SchemaResult{}, fmt.Errorf("%w: reports %q schema %d for %s", ErrManifestInvalid, m.Version, m.SchemaVersion, tag)
	}
	return SchemaResult{Version: m.Version, Schema: m.SchemaVersion, Detail: detail}, nil
}
