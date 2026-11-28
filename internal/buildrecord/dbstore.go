package buildrecord

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/glincker/levelrail/internal/store"
)

// DBStore adapts the store rows to Store and ImageStore.
type DBStore struct{ DB *store.DB }

// SaveRecord stores a signed record.
func (s DBStore) SaveRecord(ctx context.Context, id, app string, r Signed) error {
	claims, err := json.Marshal(r.Claims)
	if err != nil {
		return err
	}
	return s.DB.SaveBuildRecord(ctx, store.BuildRecord{ID: id, App: app, Claims: string(claims), Signature: r.Signature, KeyID: r.KeyID, CreatedAt: r.Claims.IssuedAt})
}

// GetRecord loads a record by id.
func (s DBStore) GetRecord(ctx context.Context, id string) (Signed, error) {
	row, err := s.DB.GetBuildRecord(ctx, id)
	if err != nil {
		return Signed{}, err
	}
	return decodeRow(row)
}

// LinkAttempt ties a record to a deploy attempt.
func (s DBStore) LinkAttempt(ctx context.Context, id, attemptID string) error {
	return s.DB.LinkBuildRecordAttempt(ctx, id, attemptID)
}

// RecordByDigest finds the newest record of a digest.
func (s DBStore) RecordByDigest(ctx context.Context, digest string) (string, Signed, error) {
	return s.decode(s.DB.GetBuildRecordByDigest(ctx, digest))
}

// RecordByRepo finds the newest record of an exact image reference.
func (s DBStore) RecordByRepo(ctx context.Context, repo string) (string, Signed, error) {
	return s.decode(s.DB.GetBuildRecordByRepo(ctx, repo))
}

func (s DBStore) decode(row store.BuildRecord, err error) (string, Signed, error) {
	if errors.Is(err, store.ErrBuildRecordNotFound) {
		return "", Signed{}, ErrNoRecord
	}
	if err != nil {
		return "", Signed{}, err
	}
	out, err := decodeRow(row)
	return row.ID, out, err
}

func decodeRow(row store.BuildRecord) (Signed, error) {
	out := Signed{Signature: row.Signature, KeyID: row.KeyID}
	if err := json.Unmarshal([]byte(row.Claims), &out.Claims); err != nil {
		return Signed{}, err
	}
	return out, nil
}
