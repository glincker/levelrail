package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Saved view sharing scopes.
const (
	SavedViewPrivate = "private"
	SavedViewTeam    = "team"
)

// ErrSavedViewNotFound is returned when no saved view has the given id.
var ErrSavedViewNotFound = errors.New("store: saved view not found")

// SavedView is a named filter of one list page, private to its owner or shared with one team.
type SavedView struct {
	ID        string
	Owner     string
	TeamID    string
	List      string
	Name      string
	Filter    string
	Shared    string
	CreatedAt string
	UpdatedAt string
}

const savedViewColumns = `id, owner, team_id, list, name, filter, shared, created_at, updated_at`

func scanSavedView(sc interface{ Scan(...any) error }) (SavedView, error) {
	var v SavedView
	err := sc.Scan(&v.ID, &v.Owner, &v.TeamID, &v.List, &v.Name, &v.Filter, &v.Shared, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// ErrSavedViewLimit is returned when a new view would exceed the owner's cap.
var ErrSavedViewLimit = errors.New("store: saved view limit reached")

// UpsertSavedView saves a view, replacing the owner's view with the same list and name.
// A new name is inserted only while the owner has fewer than maxPerOwner views, in one statement so concurrent saves cannot pass the cap.
func (db *DB) UpsertSavedView(ctx context.Context, v SavedView, maxPerOwner int) (SavedView, error) {
	now := formatTime(time.Now())
	res, err := db.ExecContext(ctx, `
		INSERT INTO saved_views (id, owner, team_id, list, name, filter, shared, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM saved_views WHERE owner = ? AND list = ? AND name = ?)
			OR (SELECT COUNT(*) FROM saved_views WHERE owner = ?) < ?
		ON CONFLICT(owner, list, name) DO UPDATE SET
			filter = excluded.filter, shared = excluded.shared, team_id = excluded.team_id, updated_at = excluded.updated_at`,
		v.ID, v.Owner, v.TeamID, v.List, v.Name, v.Filter, v.Shared, now, now,
		v.Owner, v.List, v.Name, v.Owner, maxPerOwner)
	if err != nil {
		return SavedView{}, fmt.Errorf("store: save view %q: %w", v.Name, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return SavedView{}, fmt.Errorf("store: save view %q: rows affected: %w", v.Name, err)
	} else if n == 0 {
		return SavedView{}, ErrSavedViewLimit
	}
	return db.FindSavedView(ctx, v.Owner, v.List, v.Name)
}

// FindSavedView returns the owner's view of a list by name.
func (db *DB) FindSavedView(ctx context.Context, owner, list, name string) (SavedView, error) {
	v, err := scanSavedView(db.QueryRowContext(ctx, `SELECT `+savedViewColumns+` FROM saved_views WHERE owner = ? AND list = ? AND name = ?`, owner, list, name))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedView{}, ErrSavedViewNotFound
	}
	if err != nil {
		return SavedView{}, fmt.Errorf("store: find view %q: %w", name, err)
	}
	return v, nil
}

// GetSavedView returns one view by id.
func (db *DB) GetSavedView(ctx context.Context, id string) (SavedView, error) {
	v, err := scanSavedView(db.QueryRowContext(ctx, `SELECT `+savedViewColumns+` FROM saved_views WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedView{}, ErrSavedViewNotFound
	}
	if err != nil {
		return SavedView{}, fmt.Errorf("store: get view %q: %w", id, err)
	}
	return v, nil
}

// SavedViewFilter selects the views one caller may read. AllTeams is an
// instance admin; otherwise only views of Teams are returned.
type SavedViewFilter struct {
	Owner    string
	Teams    []string
	AllTeams bool
	List     string
}

// ListSavedViews returns the caller's own views and the team shared views of its teams, by list then name.
func (db *DB) ListSavedViews(ctx context.Context, f SavedViewFilter) ([]SavedView, error) {
	teamClause := "1 = 1"
	args := []any{f.Owner}
	if !f.AllTeams {
		teamClause = "0 = 1"
		if len(f.Teams) > 0 {
			teamClause = "team_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(f.Teams)), ",") + ")"
			for _, t := range f.Teams {
				args = append(args, t)
			}
		}
	}
	// The owner's own views stay limited to visible teams too, so a narrowed token sees only its team.
	q := `SELECT ` + savedViewColumns + ` FROM saved_views WHERE (owner = ? OR shared = 'team') AND (` + teamClause + ` OR team_id = '')`
	if f.List != "" {
		q += ` AND list = ?`
		args = append(args, f.List)
	}
	q += ` ORDER BY list, name, id`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list views: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SavedView
	for rows.Next() {
		v, err := scanSavedView(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan view: %w", err)
		}
		if v.Owner != f.Owner && (v.Shared != SavedViewTeam || v.TeamID == "") {
			continue
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate views: %w", err)
	}
	return out, nil
}

// CountSavedViews returns how many views one owner has.
func (db *DB) CountSavedViews(ctx context.Context, owner string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM saved_views WHERE owner = ?`, owner).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count views: %w", err)
	}
	return n, nil
}

// DeleteSavedView removes one view by id.
func (db *DB) DeleteSavedView(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM saved_views WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete view %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrSavedViewNotFound, "delete view %q", id)
}
