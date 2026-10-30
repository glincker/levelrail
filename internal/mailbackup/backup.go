package mailbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"time"
)

// Backup runs mox backup into a staging dir, verifies it, then streams the
// archive and manifest to the sink. Staging is removed on every path.
func Backup(ctx context.Context, spec BackupSpec) (Manifest, error) {
	now := time.Now
	if spec.Now != nil {
		now = spec.Now
	}
	created := now().UTC()
	id := created.Format("20060102T150405Z")
	host := spec.StagingHost + string(os.PathSeparator) + id
	cont := path.Join(spec.StagingContainer, id)
	defer func() { _ = os.RemoveAll(host) }()

	verOut, err := spec.Runner.Run(ctx, "version")
	if err != nil {
		return Manifest{}, fmt.Errorf("mox version: %w", err)
	}
	_, ver, err := parseVersion(verOut)
	if err != nil {
		return Manifest{}, err
	}
	if _, err := spec.Runner.Run(ctx, "backup", cont); err != nil {
		return Manifest{}, fmt.Errorf("mox backup: %w", err)
	}
	if _, err := spec.Runner.Run(ctx, "verifydata", path.Join(cont, "data")); err != nil {
		return Manifest{}, fmt.Errorf("mox verifydata: %w", err)
	}
	files, size, err := scanDir(host)
	if err != nil {
		return Manifest{}, fmt.Errorf("scan staging: %w", err)
	}

	sum := sha256.New()
	pr, pw := io.Pipe()
	go func() {
		_ = pw.CloseWithError(writeArchive(host, io.MultiWriter(pw, sum), spec.Recipients))
	}()
	putErr := spec.Sink.Put(ctx, archiveName(id), pr)
	// Unblocks the writer goroutine when the sink stopped reading early.
	_ = pr.CloseWithError(putErr)
	if putErr != nil {
		_ = spec.Sink.Delete(ctx, archiveName(id))
		return Manifest{}, putErr
	}

	m := Manifest{
		ID: id, MoxVersion: ver, CreatedAt: created, Files: files, Bytes: size,
		SHA256: hex.EncodeToString(sum.Sum(nil)), ArchiveName: archiveName(id), Verified: true,
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return Manifest{}, err
	}
	if err := spec.Sink.Put(ctx, manifestName(id), bytes.NewReader(raw)); err != nil {
		_ = spec.Sink.Delete(ctx, archiveName(id))
		return Manifest{}, err
	}
	if p, ok := spec.Sink.(Pruner); ok {
		if _, err := p.Prune(ctx); err != nil {
			return m, err
		}
	}
	return m, nil
}
