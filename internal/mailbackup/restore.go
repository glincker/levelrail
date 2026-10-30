package mailbackup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Restore checks the downgrade guard and the archive checksum, then either
// reports what it would extract (dryRun) or extracts into DestDir.
func Restore(ctx context.Context, spec RestoreSpec, dryRun bool) (RestoreReport, error) {
	rep := RestoreReport{DryRun: dryRun}
	mrc, err := spec.Sink.Open(ctx, manifestName(spec.ID))
	if err != nil {
		return rep, fmt.Errorf("open manifest: %w", err)
	}
	err = json.NewDecoder(mrc).Decode(&rep.Manifest)
	_ = mrc.Close()
	if err != nil {
		return rep, fmt.Errorf("read manifest: %w", err)
	}

	out, err := spec.Runner.Run(ctx, "version")
	if err != nil {
		return rep, fmt.Errorf("mox version: %w", err)
	}
	target, targetStr, err := parseVersion(out)
	if err != nil {
		return rep, err
	}
	have, _, err := parseVersion(rep.Manifest.MoxVersion)
	if err != nil {
		return rep, err
	}
	if target.less(have) {
		return rep, &DowngradeError{Target: targetStr, Backup: rep.Manifest.MoxVersion}
	}

	if err := verifyChecksum(ctx, spec, rep.Manifest); err != nil {
		return rep, err
	}

	// Extract beside DestDir and rename on success, so a failed restore
	// never leaves a half populated target.
	var work string
	if !dryRun {
		if ents, err := os.ReadDir(spec.DestDir); err == nil && len(ents) > 0 {
			return rep, fmt.Errorf("restore target %s is not empty", spec.DestDir)
		}
		work = filepath.Clean(spec.DestDir) + ".restoring"
		_ = os.RemoveAll(work)
		if err := os.MkdirAll(work, 0o700); err != nil {
			return rep, err
		}
		defer func() { _ = os.RemoveAll(work) }()
	}

	rc, err := spec.Sink.Open(ctx, manifestArchive(rep.Manifest))
	if err != nil {
		return rep, fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = rc.Close() }()
	err = readArchive(rc, spec.Identities, func(h *tar.Header, body io.Reader) error {
		if h.Typeflag == tar.TypeDir {
			if dryRun {
				return nil
			}
			p, err := safeJoin(work, h.Name)
			if err != nil {
				return err
			}
			return os.MkdirAll(p, 0o700)
		}
		if h.Typeflag != tar.TypeReg {
			return nil
		}
		rep.Files++
		if dryRun {
			n, err := io.Copy(io.Discard, body)
			rep.Bytes += n
			return err
		}
		p, err := safeJoin(work, h.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(h.Mode)&0o777|0o600) //nolint:gosec // p is confined by safeJoin
		if err != nil {
			return err
		}
		n, err := io.Copy(f, body)
		rep.Bytes += n
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if err != nil {
		return rep, fmt.Errorf("extract: %w", err)
	}
	if dryRun {
		return rep, nil
	}
	_ = os.Remove(spec.DestDir)
	if err := os.Rename(work, spec.DestDir); err != nil {
		return rep, err
	}
	return rep, nil
}

func manifestArchive(m Manifest) string { return m.ArchiveName }

func verifyChecksum(ctx context.Context, spec RestoreSpec, m Manifest) error {
	rc, err := spec.Sink.Open(ctx, m.ArchiveName)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != m.SHA256 {
		return &ChecksumError{Want: m.SHA256, Got: got}
	}
	return nil
}
