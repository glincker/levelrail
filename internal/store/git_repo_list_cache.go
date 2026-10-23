package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrGitRepoListCacheMiss means no cached repository list exists for the key.
var ErrGitRepoListCacheMiss = errors.New("store: git repo list cache miss")

// GitRepoListCacheEntry is one connection's cached repository list. Payload
// is opaque JSON owned by internal/repolist; Meta carries provider-specific
// revalidation data (GitHub page ETags).
type GitRepoListCacheEntry struct {
	Key       string
	Provider  string
	Payload   string
	Meta      string
	Truncated bool
	FetchedAt time.Time
}

// GetGitRepoListCache loads the cached list for key.
func (db *DB) GetGitRepoListCache(ctx context.Context, key string) (GitRepoListCacheEntry, error) {
	var (
		e         GitRepoListCacheEntry
		truncated int
		fetched   string
	)
	err := db.QueryRowContext(ctx, `
		SELECT cache_key, provider, payload, meta, truncated, fetched_at
		FROM git_repo_list_cache WHERE cache_key = ?`, key).
		Scan(&e.Key, &e.Provider, &e.Payload, &e.Meta, &truncated, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return GitRepoListCacheEntry{}, ErrGitRepoListCacheMiss
	}
	if err != nil {
		return GitRepoListCacheEntry{}, fmt.Errorf("store: get git repo list cache %q: %w", key, err)
	}
	e.Truncated = truncated != 0
	e.FetchedAt = parseOptionalTime(fetched)
	return e, nil
}

// PutGitRepoListCache inserts or replaces the cached list for e.Key.
func (db *DB) PutGitRepoListCache(ctx context.Context, e GitRepoListCacheEntry) error {
	truncated := 0
	if e.Truncated {
		truncated = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO git_repo_list_cache (cache_key, provider, payload, meta, truncated, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET provider = excluded.provider, payload = excluded.payload,
			meta = excluded.meta, truncated = excluded.truncated, fetched_at = excluded.fetched_at`,
		e.Key, e.Provider, e.Payload, e.Meta, truncated, e.FetchedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: put git repo list cache %q: %w", e.Key, err)
	}
	return nil
}

// DeleteGitRepoListCachePrefix drops every cached list whose key starts with prefix.
func (db *DB) DeleteGitRepoListCachePrefix(ctx context.Context, prefix string) error {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	if _, err := db.ExecContext(ctx, `DELETE FROM git_repo_list_cache WHERE cache_key LIKE ? ESCAPE '\'`, esc+"%"); err != nil {
		return fmt.Errorf("store: delete git repo list cache %q: %w", prefix, err)
	}
	return nil
}
