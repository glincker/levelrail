package plugin

import (
	"context"
	"database/sql"
	"errors"
)

// Source is one signed index an operator trusts to list plugins.
type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	RootKey  string `json:"root_public_key"`
	Builtin  bool   `json:"builtin"`
	AddedAt  string `json:"added_at"`
	AddedBy  string `json:"added_by"`
	KeyPrint string `json:"root_key_id"`
}

// SourceState is what the control plane remembers about a source between fetches.
type SourceState struct {
	SourceID      string
	LastSequence  int64
	LastGenerated string
	LastHash      string
	ETag          string
	Body          string
	FetchedAt     string
	LastError     string
	ErrorKind     string
}

// MarketStore persists sources and their fetch state.
type MarketStore interface {
	ListSources(ctx context.Context) ([]Source, error)
	AddSource(ctx context.Context, s Source) error
	RemoveSource(ctx context.Context, id string) error
	GetState(ctx context.Context, sourceID string) (SourceState, error)
	SaveState(ctx context.Context, st SourceState) error
}

// ErrSourceUnknown marks a source id that is not configured.
var ErrSourceUnknown = errors.New("plugin: marketplace source not found")

// ErrSourceExists marks a source that is already configured.
var ErrSourceExists = errors.New("plugin: marketplace source already configured")

// SQLMarketStore implements MarketStore over the control plane database.
type SQLMarketStore struct{ db DBTX }

// NewSQLMarketStore wraps db.
func NewSQLMarketStore(db DBTX) *SQLMarketStore { return &SQLMarketStore{db: db} }

// ListSources returns the operator-added sources, oldest first.
func (s *SQLMarketStore) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, root_public_key, added_at, added_by FROM plugin_marketplace_sources ORDER BY added_at, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Source{}
	for rows.Next() {
		var r Source
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.RootKey, &r.AddedAt, &r.AddedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddSource inserts a source, or returns ErrSourceExists.
func (s *SQLMarketStore) AddSource(ctx context.Context, r Source) error {
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO plugin_marketplace_sources (id, name, url, root_public_key, added_at, added_by) VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.URL, r.RootKey, r.AddedAt, r.AddedBy)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceExists
	}
	return nil
}

// RemoveSource deletes a source and its fetch state.
func (s *SQLMarketStore) RemoveSource(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugin_marketplace_sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceUnknown
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM plugin_marketplace_state WHERE source_id = ?`, id)
	return err
}

// GetState returns the saved state, or a zero state for a source never fetched.
func (s *SQLMarketStore) GetState(ctx context.Context, sourceID string) (SourceState, error) {
	st := SourceState{SourceID: sourceID}
	err := s.db.QueryRowContext(ctx, `SELECT last_sequence, last_generated_at, last_hash, etag, body, fetched_at, last_error, error_kind FROM plugin_marketplace_state WHERE source_id = ?`, sourceID).
		Scan(&st.LastSequence, &st.LastGenerated, &st.LastHash, &st.ETag, &st.Body, &st.FetchedAt, &st.LastError, &st.ErrorKind)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	return st, err
}

// SaveState upserts the state row.
func (s *SQLMarketStore) SaveState(ctx context.Context, st SourceState) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO plugin_marketplace_state (source_id, last_sequence, last_generated_at, last_hash, etag, body, fetched_at, last_error, error_kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET last_sequence = excluded.last_sequence, last_generated_at = excluded.last_generated_at, last_hash = excluded.last_hash,
		etag = excluded.etag, body = excluded.body, fetched_at = excluded.fetched_at, last_error = excluded.last_error, error_kind = excluded.error_kind`,
		st.SourceID, st.LastSequence, st.LastGenerated, st.LastHash, st.ETag, st.Body, st.FetchedAt, st.LastError, st.ErrorKind)
	return err
}
