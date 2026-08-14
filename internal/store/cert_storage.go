package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCertStorageKeyNotFound is returned by GetCertStorageValue and
// StatCertStorageValue when key has no stored value. Named distinctly
// from ErrServiceNotFound/ErrNodeNotFound, even though the shape is
// identical, because internal/ingress's certmagic.Storage adapter
// (TASKS.md 3.6) needs to translate this into fs.ErrNotExist
// specifically, per certmagic.Storage's own doc comment ("Load, Delete,
// List, and Stat methods should return fs.ErrNotExist if the key does
// not exist"); a store-package-wide NotFound sentinel would make that
// translation ambiguous about which resource type failed.
var ErrCertStorageKeyNotFound = errors.New("store: cert storage key not found")

// CertStorageValue is one stored key's value and last-write time.
type CertStorageValue struct {
	Value      []byte
	ModifiedAt time.Time
}

// CertStorageKeyInfo mirrors certmagic.KeyInfo's shape (see
// internal/ingress.SQLiteStorage.Stat), decoupled from the certmagic
// import so this package never needs to depend on Caddy/certmagic
// itself, only internal/ingress does.
type CertStorageKeyInfo struct {
	Key        string
	ModifiedAt time.Time
	Size       int64
	// IsTerminal is true for an exact stored value ("file"), false when
	// key only exists as a prefix of other stored keys ("directory"),
	// matching certmagic.Storage's own file-system-shaped key semantics.
	IsTerminal bool
}

// SaveCertStorageValue writes value at key, creating or overwriting.
// Backs certmagic.Storage's Store method (internal/ingress, TASKS.md
// 3.6): Caddy's TLS automation calls this to persist ACME account keys,
// issued certificates, and OCSP staples.
func (db *DB) SaveCertStorageValue(ctx context.Context, key string, value []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO cert_storage (key, value, modified_at)
		VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			value = excluded.value,
			modified_at = excluded.modified_at
	`, key, value, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save cert storage value %q: %w", key, err)
	}
	return nil
}

// GetCertStorageValue returns the value stored at key, or
// ErrCertStorageKeyNotFound.
func (db *DB) GetCertStorageValue(ctx context.Context, key string) (*CertStorageValue, error) {
	row := db.QueryRowContext(ctx, `SELECT value, modified_at FROM cert_storage WHERE key = ?`, key)
	var (
		value         []byte
		modifiedAtStr string
	)
	err := row.Scan(&value, &modifiedAtStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCertStorageKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get cert storage value %q: %w", key, err)
	}
	modifiedAt, err := time.Parse(time.RFC3339Nano, modifiedAtStr)
	if err != nil {
		return nil, fmt.Errorf("store: parse modified_at for cert storage value %q: %w", key, err)
	}
	return &CertStorageValue{Value: value, ModifiedAt: modifiedAt}, nil
}

// DeleteCertStorageValue removes key. certmagic.Storage.Delete's doc
// comment requires deleting every key prefixed by key too ("directory"
// semantics), so this also removes any key under key+"/", not only an
// exact match. Deleting a key that exists as neither is not an error,
// matching certmagic's own FileStorage.Delete (os.RemoveAll on an
// already-absent path is a no-op).
func (db *DB) DeleteCertStorageValue(ctx context.Context, key string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM cert_storage WHERE key = ? OR key LIKE ? ESCAPE '\'
	`, key, likePrefixPattern(key+"/")); err != nil {
		return fmt.Errorf("store: delete cert storage value %q: %w", key, err)
	}
	return nil
}

// ExistsCertStorageValue reports whether key is stored either as an
// exact value or as a "directory" prefix of other stored keys.
func (db *DB) ExistsCertStorageValue(ctx context.Context, key string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM cert_storage WHERE key = ? OR key LIKE ? ESCAPE '\' LIMIT 1
	`, key, likePrefixPattern(key+"/")).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check cert storage key %q exists: %w", key, err)
	}
	return true, nil
}

// ListCertStorageKeys returns every stored key prefixed by prefix/. If
// recursive is false, only the immediate next path segment after prefix
// is returned once per distinct value: certmagic's "directory listing"
// semantics, where a partial key up to the next "/" stands in for every
// key nested further below it, matching certmagic.FileStorage.List's
// non-recursive behavior of listing directory entries rather than every
// file transitively inside them. Returns an empty slice, not an error,
// when nothing matches; internal/ingress's adapter is the layer that
// translates "nothing found" into fs.ErrNotExist, per certmagic's
// contract, so this stays a plain query with no certmagic-specific
// knowledge.
func (db *DB) ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	// An empty prefix means "list from the root": every key matches,
	// there is no literal "prefix/" boundary to require. certmagic never
	// actually calls List with an empty prefix itself (every call site
	// passes a fixed, non-empty storage-key namespace), but handling it
	// correctly rather than returning nothing keeps this method's
	// contract honest on its own terms.
	dirPrefix := ""
	pattern := "%"
	if prefix != "" {
		dirPrefix = prefix + "/"
		pattern = likePrefixPattern(dirPrefix)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT key FROM cert_storage WHERE key LIKE ? ESCAPE '\' ORDER BY key
	`, pattern)
	if err != nil {
		return nil, fmt.Errorf("store: list cert storage keys prefixed %q: %w", prefix, err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("store: scan cert storage key row: %w", err)
		}
		entry := key
		if !recursive {
			rest := strings.TrimPrefix(key, dirPrefix)
			if idx := strings.IndexByte(rest, '/'); idx >= 0 {
				entry = dirPrefix + rest[:idx]
			}
		}
		if !seen[entry] {
			seen[entry] = true
			out = append(out, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate cert storage key rows: %w", err)
	}
	return out, nil
}

// StatCertStorageValue returns key's metadata, or
// ErrCertStorageKeyNotFound if key is stored neither as an exact value
// nor as a directory prefix of other keys.
func (db *DB) StatCertStorageValue(ctx context.Context, key string) (*CertStorageKeyInfo, error) {
	row := db.QueryRowContext(ctx, `SELECT value, modified_at FROM cert_storage WHERE key = ?`, key)
	var (
		value         []byte
		modifiedAtStr string
	)
	err := row.Scan(&value, &modifiedAtStr)
	if err == nil {
		modifiedAt, perr := time.Parse(time.RFC3339Nano, modifiedAtStr)
		if perr != nil {
			return nil, fmt.Errorf("store: parse modified_at for cert storage value %q: %w", key, perr)
		}
		return &CertStorageKeyInfo{Key: key, ModifiedAt: modifiedAt, Size: int64(len(value)), IsTerminal: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: stat cert storage value %q: %w", key, err)
	}

	exists, existsErr := db.ExistsCertStorageValue(ctx, key)
	if existsErr != nil {
		return nil, fmt.Errorf("store: stat cert storage value %q: %w", key, existsErr)
	}
	if !exists {
		return nil, ErrCertStorageKeyNotFound
	}
	// key is a "directory": a prefix of other stored keys but has no
	// value of its own. There is no single row to derive Modified/Size
	// from (unlike a real filesystem directory inode), so those are left
	// zero; certmagic only relies on IsTerminal to distinguish this case,
	// per its own Stat doc comment.
	return &CertStorageKeyInfo{Key: key, IsTerminal: false}, nil
}

// AcquireCertStorageLock attempts to atomically claim the named lock:
// insert it if free, or steal it if the existing holder's last refresh
// is older than staleAfter (an abandoned lock from a holder that
// crashed without calling ReleaseCertStorageLock). Returns false, nil
// (not an error) if a different, still-fresh holder currently has the
// lock; the caller is expected to wait and retry, the same shape
// certmagic.FileStorage.Lock's own polling loop uses.
func (db *DB) AcquireCertStorageLock(ctx context.Context, name string, staleAfter time.Duration) (bool, error) {
	now := time.Now()
	nowStr := formatTime(now)
	staleBefore := formatTime(now.Add(-staleAfter))

	res, err := db.ExecContext(ctx, `
		INSERT INTO cert_storage_locks (name, acquired_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET
			acquired_at = excluded.acquired_at,
			updated_at = excluded.updated_at
		WHERE cert_storage_locks.updated_at < ?
	`, name, nowStr, nowStr, staleBefore)
	if err != nil {
		return false, fmt.Errorf("store: acquire cert storage lock %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: acquire cert storage lock %q: rows affected: %w", name, err)
	}
	return n > 0, nil
}

// TouchCertStorageLock refreshes name's lock's updated_at to now, so a
// live holder's lock never looks stale to a competitor calling
// AcquireCertStorageLock. Best-effort by design, matching
// TouchNodeLastSeen and TouchAPITokenLastUsed: a missed refresh is not
// itself fatal to the holder's in-progress operation, only a risk that a
// competitor could (rarely) steal the lock early if staleAfter is also
// exceeded, which is an accepted, documented trade-off of the whole
// staleness-based design (see 0010's migration comment).
func (db *DB) TouchCertStorageLock(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `
		UPDATE cert_storage_locks SET updated_at = ? WHERE name = ?
	`, formatTime(time.Now()), name); err != nil {
		return fmt.Errorf("store: touch cert storage lock %q: %w", name, err)
	}
	return nil
}

// ReleaseCertStorageLock removes name's lock. Idempotent: releasing an
// already-released or never-held lock is not an error, matching
// DeleteNode's convention for the same reason (Unlock is often called
// from a defer alongside error paths that may have already cleaned up).
func (db *DB) ReleaseCertStorageLock(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM cert_storage_locks WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: release cert storage lock %q: %w", name, err)
	}
	return nil
}

// likePrefixPattern escapes SQLite LIKE's wildcard characters ('%', '_')
// and the escape character itself in s, then appends '%' so the result
// matches every key with s as a literal (non-wildcard) prefix. Domain
// and ACME storage keys don't normally contain '%' or '_', but escaping
// unconditionally means a key that did wouldn't silently corrupt a
// prefix scan into matching unrelated keys.
func likePrefixPattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s) + "%"
}
