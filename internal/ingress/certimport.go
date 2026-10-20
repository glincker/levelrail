package ingress

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/caddyserver/caddy/v2"
)

// certImportRoots are the certmagic storage prefixes worth carrying over:
// issued certificates and ACME account keys. Locks are transient.
var certImportRoots = []string{"certificates", "acme"}

// ImportFileCerts copies a legacy Caddy file-storage tree (dir/certificates,
// dir/acme) into the database-backed cert storage, skipping keys already
// present, so switching storage does not force every domain to re-issue.
// A missing dir is not an error. Returns the number of keys imported.
func ImportFileCerts(ctx context.Context, certStore CertStore, dir string, logger *slog.Logger) (int, error) {
	if logger == nil {
		logger = slog.Default()
	}
	imported := 0
	for _, root := range certImportRoots {
		base := filepath.Join(dir, root)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, relErr := filepath.Rel(dir, p)
			if relErr != nil {
				return fmt.Errorf("rel path %q: %w", p, relErr)
			}
			key := filepath.ToSlash(rel)
			exists, existsErr := certStore.ExistsCertStorageValue(ctx, key)
			if existsErr != nil {
				return fmt.Errorf("check %q: %w", key, existsErr)
			}
			if exists || strings.HasSuffix(key, ".lock") {
				return nil
			}
			data, readErr := os.ReadFile(p) // #nosec G304 G122 -- walking Caddy's own storage dir
			if readErr != nil {
				return fmt.Errorf("read %q: %w", p, readErr)
			}
			if saveErr := certStore.SaveCertStorageValue(ctx, key, data); saveErr != nil {
				return fmt.Errorf("save %q: %w", key, saveErr)
			}
			imported++
			return nil
		})
		if walkErr != nil {
			return imported, fmt.Errorf("ingress: import file certs from %q: %w", base, walkErr)
		}
	}
	if imported > 0 {
		logger.Info("imported legacy file certificate storage into the database", slog.Int("keys", imported))
	}
	return imported, nil
}

// LegacyStorageDir is where Caddy's default file storage keeps its data
// when no storage module is configured.
func LegacyStorageDir() string { return caddy.AppDataDir() }
