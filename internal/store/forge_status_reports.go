package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// Sources of a forge status report.
const (
	ForgeStatusSourcePipeline = "pipeline"
	ForgeStatusSourcePreview  = "preview"
)

const (
	forgeStatusIDPrefix    = "fsr_"
	forgeStatusTextMax     = 500
	forgeStatusListDefault = 50
)

// ForgeStatusReport is one commit status Levelrail posted to a forge. Error
// is the failure text when the post did not go through, never a token.
type ForgeStatusReport struct {
	ID          string
	AppName     string
	Provider    string
	SHA         string
	Context     string
	State       string
	Description string
	TargetURL   string
	Source      string
	Error       string
	CreatedAt   time.Time
}

// RecordForgeStatusReport stores one report. An empty ID and a zero
// CreatedAt are filled in.
func (db *DB) RecordForgeStatusReport(ctx context.Context, r ForgeStatusReport) error {
	if r.AppName == "" || r.SHA == "" || r.State == "" {
		return fmt.Errorf("store: forge status report needs app, sha and state")
	}
	if r.ID == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Errorf("store: forge status id: %w", err)
		}
		r.ID = forgeStatusIDPrefix + hex.EncodeToString(b[:])
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO forge_status_reports (id, app_name, provider, sha, context, state, description, target_url, source, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AppName, r.Provider, r.SHA, r.Context, r.State,
		clipForgeStatusText(secretmask.Mask(r.Description)), r.TargetURL, r.Source, clipForgeStatusText(secretmask.Mask(r.Error)),
		r.CreatedAt.UTC().Format(appEventTimeFormat))
	if err != nil {
		return fmt.Errorf("store: record forge status for %q: %w", r.AppName, err)
	}
	return nil
}

// ListForgeStatusReports returns an app's reports newest first, resuming
// after cursor when set. limit <= 0 uses the default.
func (db *DB) ListForgeStatusReports(ctx context.Context, app string, cursor *AppEventCursor, limit int) ([]ForgeStatusReport, error) {
	if limit <= 0 {
		limit = forgeStatusListDefault
	}
	q := `SELECT id, app_name, provider, sha, context, state, description, target_url, source, error, created_at
		FROM forge_status_reports WHERE app_name = ?`
	args := []any{app}
	if cursor != nil {
		at := cursor.At.UTC().Format(appEventTimeFormat)
		q += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, at, at, cursor.ID)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list forge status for %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	out := []ForgeStatusReport{}
	for rows.Next() {
		var (
			r  ForgeStatusReport
			at string
		)
		if err := rows.Scan(&r.ID, &r.AppName, &r.Provider, &r.SHA, &r.Context, &r.State, &r.Description, &r.TargetURL, &r.Source, &r.Error, &at); err != nil {
			return nil, fmt.Errorf("store: scan forge status: %w", err)
		}
		t, err := time.Parse(appEventTimeFormat, at)
		if err != nil {
			return nil, fmt.Errorf("store: parse forge status time %q: %w", at, err)
		}
		r.CreatedAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}

func clipForgeStatusText(s string) string {
	if len(s) <= forgeStatusTextMax {
		return s
	}
	return s[:forgeStatusTextMax]
}
