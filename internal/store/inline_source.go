package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ErrInlineSourceNotFound is returned when no inline source with that hash exists for the app.
var ErrInlineSourceNotFound = errors.New("store: inline source not found")

// InlineFile is one extra build context file stored next to an inline Dockerfile.
type InlineFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// InlineSource is a pasted Dockerfile plus optional context files, the
// no-git counterpart of a GitSource's BuildType and BuildPath.
type InlineSource struct {
	ServiceName string
	Hash        string
	Dockerfile  string
	Files       []InlineFile
	SizeBytes   int64
	CreatedAt   string
}

// InlineSourceHash is the sha256 of the Dockerfile and files with length
// prefixes, so two different splits of the same bytes never collide.
func InlineSourceHash(dockerfile string, files []InlineFile) string {
	h := sha256.New()
	write := func(s string) {
		_, _ = h.Write([]byte(strconv.Itoa(len(s)) + ":"))
		_, _ = h.Write([]byte(s))
	}
	write(dockerfile)
	for _, f := range files {
		write(f.Path)
		write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// InlineSourceSize is the byte total that the size cap applies to.
func InlineSourceSize(dockerfile string, files []InlineFile) int64 {
	n := int64(len(dockerfile))
	for _, f := range files {
		n += int64(len(f.Path) + len(f.Content))
	}
	return n
}

// SaveInlineSource stores src under its content hash and returns that
// hash; saving identical content again is a no-op.
func (db *DB) SaveInlineSource(ctx context.Context, src InlineSource) (string, error) {
	files := src.Files
	if files == nil {
		files = []InlineFile{}
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return "", fmt.Errorf("store: save inline source for %q: %w", src.ServiceName, err)
	}
	hash := InlineSourceHash(src.Dockerfile, files)
	_, err = db.ExecContext(ctx, `
		INSERT INTO app_inline_sources (service_name, hash, dockerfile, files, size_bytes, file_count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, hash) DO UPDATE SET created_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, src.ServiceName, hash, src.Dockerfile, string(filesJSON), InlineSourceSize(src.Dockerfile, files), len(files))
	if err != nil {
		return "", fmt.Errorf("store: save inline source for %q: %w", src.ServiceName, err)
	}
	return hash, nil
}

// PruneInlineSources keeps the newest keep sources of each app, plus any
// source a queued, held, pending or running attempt still has to build.
func (db *DB) PruneInlineSources(ctx context.Context, keep int) (int64, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM app_inline_sources WHERE rowid IN (
			SELECT s.rowid FROM (
				SELECT rowid, service_name, hash,
					ROW_NUMBER() OVER (PARTITION BY service_name ORDER BY created_at DESC, rowid DESC) AS rn
				FROM app_inline_sources
			) s
			WHERE s.rn > ? AND NOT EXISTS (
				SELECT 1 FROM deploy_attempts a
				WHERE a.service_name = s.service_name AND a.commit_sha = substr(s.hash, 1, 40)
					AND a.status IN (?, ?, ?, ?, ?)
			)
		)
	`, keep, DeployAttemptStatusQueued, DeployAttemptStatusHeld, DeployAttemptStatusPending, DeployAttemptStatusRunning, DeployAttemptStatusAwaitingApproval)
	if err != nil {
		return 0, fmt.Errorf("store: prune inline sources: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: prune inline sources: %w", err)
	}
	return n, nil
}

// GetInlineSource returns the app's inline source with that hash, or ErrInlineSourceNotFound.
func (db *DB) GetInlineSource(ctx context.Context, serviceName, hash string) (*InlineSource, error) {
	var src InlineSource
	var filesJSON string
	err := db.QueryRowContext(ctx, `
		SELECT service_name, hash, dockerfile, files, size_bytes, created_at
		FROM app_inline_sources WHERE service_name = ? AND hash = ?
	`, serviceName, hash).Scan(&src.ServiceName, &src.Hash, &src.Dockerfile, &filesJSON, &src.SizeBytes, &src.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInlineSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get inline source for %q: %w", serviceName, err)
	}
	if err := json.Unmarshal([]byte(filesJSON), &src.Files); err != nil {
		return nil, fmt.Errorf("store: decode inline source files for %q: %w", serviceName, err)
	}
	return &src, nil
}
