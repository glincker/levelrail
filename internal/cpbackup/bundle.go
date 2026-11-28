package cpbackup

import (
	"archive/tar"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// AlertingFile is the alert rules and channels database next to the control plane database.
	AlertingFile = "alerting.db"
	mainEntry    = "levelrail.db"
	// bundleTar marks a payload that is a tar of the control plane and alerting databases.
	bundleTar = "tar"
)

// snapshotAlerting copies alerting.db under dataDir to dest with VACUUM INTO.
// A missing file (feature unused) is not an error and reports false.
func snapshotAlerting(ctx context.Context, dataDir, dest string) (bool, error) {
	src := filepath.Join(dataDir, AlertingFile)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat %s: %w", AlertingFile, err)
	}
	db, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return false, fmt.Errorf("open %s: %w", AlertingFile, err)
	}
	defer func() { _ = db.Close() }()
	// VACUUM INTO takes no bind parameters; dest is built by this package and quote-escaped.
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(dest, "'", "''")+"'"); err != nil { //nolint:gosec // see above
		_ = os.Remove(dest)
		return false, fmt.Errorf("snapshot %s: %w", AlertingFile, err)
	}
	if err := checkIntegrity(ctx, dest); err != nil {
		_ = os.Remove(dest)
		return false, err
	}
	return true, nil
}

func checkIntegrity(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = db.Close() }()
	var res string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return fmt.Errorf("integrity check %s: %w", filepath.Base(path), err)
	}
	if res != "ok" {
		return fmt.Errorf("integrity check %s failed: %s", filepath.Base(path), res)
	}
	return nil
}

// packBundle writes a tar of the given files to out. Headers are fixed so equal
// content gives equal bytes, which the replica relies on to skip unchanged ships.
func packBundle(out string, files map[string]string) error {
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path built by this package
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	tw := tar.NewWriter(f)
	for _, name := range []string{mainEntry, AlertingFile} {
		src, ok := files[name]
		if !ok {
			continue
		}
		if err := addEntry(tw, name, src); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = f.Close()
		return fmt.Errorf("finish bundle: %w", err)
	}
	return f.Close()
}

func addEntry(tw *tar.Writer, name, src string) error {
	in, err := os.Open(src) //nolint:gosec // path built by this package
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = in.Close() }()
	fi, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: fi.Size(), Typeflag: tar.TypeReg}); err != nil {
		return fmt.Errorf("bundle %s: %w", name, err)
	}
	if _, err := io.Copy(tw, in); err != nil {
		return fmt.Errorf("bundle %s: %w", name, err)
	}
	return nil
}

// unpackBundle extracts the two known entries into dir, reading at most maxBytes in total.
// It returns the alerting path, or "" when the bundle held none.
func unpackBundle(bundle, dir string, maxBytes int64) (mainPath, alertPath string, err error) {
	in, err := os.Open(bundle) //nolint:gosec // path built by this package
	if err != nil {
		return "", "", fmt.Errorf("open bundle: %w", err)
	}
	defer func() { _ = in.Close() }()
	tr := tar.NewReader(io.LimitReader(in, maxBytes+1))
	remaining := maxBytes
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("read bundle: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			return "", "", fmt.Errorf("backup bundle entry %q is not a regular file", h.Name)
		}
		if h.Size < 0 || h.Size > remaining {
			return "", "", errors.New("backup bundle is larger than its manifest declares")
		}
		remaining -= h.Size
		var dst *string
		switch h.Name {
		case mainEntry:
			dst = &mainPath
		case AlertingFile:
			dst = &alertPath
		default:
			return "", "", fmt.Errorf("unexpected entry %q in backup bundle", h.Name)
		}
		if *dst != "" {
			return "", "", fmt.Errorf("duplicate entry %q in backup bundle", h.Name)
		}
		*dst = filepath.Join(dir, "restored-"+h.Name)
		if err := writeLimited(*dst, tr, h.Size); err != nil {
			return "", "", err
		}
	}
	if mainPath == "" {
		return "", "", errors.New("backup bundle holds no control plane database")
	}
	return mainPath, alertPath, nil
}

// payload is what one backup run encrypts: the control plane snapshot, plus alerting.db when present.
type payload struct {
	path        string
	snapshot    string
	hasAlerting bool
	// alertingWarning is set when alerting.db exists but could not be snapshotted.
	alertingWarning string
	bundleKind      string
	schemaVersion   int
	migrations      int
	instanceID      string
}

// buildPayload snapshots the databases into tmp and returns the file to seal.
func (s *Service) buildPayload(ctx context.Context, tmp string) (payload, error) {
	p := payload{snapshot: filepath.Join(tmp, "snapshot.db")}
	if _, _, err := s.DB.SnapshotTo(ctx, p.snapshot); err != nil {
		return p, fmt.Errorf("snapshot: %w", err)
	}
	var err error
	if p.schemaVersion, err = store.InspectSnapshot(ctx, p.snapshot); err != nil {
		return p, fmt.Errorf("inspect snapshot: %w", err)
	}
	if p.migrations, err = inspectMigrationCount(ctx, p.snapshot); err != nil {
		return p, err
	}
	p.instanceID = readInstanceID(ctx, p.snapshot)
	alertSnap := filepath.Join(tmp, "alerting-snapshot.db")
	has, err := snapshotAlerting(ctx, s.DataDir, alertSnap)
	if err != nil {
		// A broken alerting.db must not stop the control plane database from being backed up.
		p.alertingWarning = err.Error()
		s.logger().Warn("alerting.db left out of the backup", slog.String("error", err.Error()))
		has = false
	}
	if !has {
		p.path = p.snapshot
		return p, nil
	}
	p.hasAlerting, p.bundleKind = true, bundleTar
	p.path = filepath.Join(tmp, "bundle.tar")
	if err := packBundle(p.path, map[string]string{mainEntry: p.snapshot, AlertingFile: alertSnap}); err != nil {
		return p, err
	}
	return p, nil
}
