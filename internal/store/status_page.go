package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// StatusPageSettings is the single settings row for the public status page.
type StatusPageSettings struct {
	Enabled      bool
	Title        string
	Description  string
	CustomDomain string
	UpdatedAt    time.Time
}

// StatusComponent is one operator-chosen thing shown on the status page.
// Target is internal; only DisplayName is ever published.
type StatusComponent struct {
	ID          string
	Kind        string
	Target      string
	DisplayName string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// StatusDaily is one component's probe counters for one UTC day.
type StatusDaily struct {
	ComponentID string
	Day         string
	OK          int
	Degraded    int
	Down        int
}

// StatusIncident is an operator-authored incident or maintenance announcement.
type StatusIncident struct {
	ID           string
	Kind         string
	Title        string
	Status       string
	Impact       string
	ComponentIDs []string
	StartsAt     time.Time
	EndsAt       *time.Time
	ResolvedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StatusIncidentUpdate is one timeline entry on an incident.
type StatusIncidentUpdate struct {
	ID         string
	IncidentID string
	Status     string
	Body       string
	CreatedAt  time.Time
}

// Sentinel errors for the status page store.
var (
	ErrStatusComponentNotFound = errors.New("store: status component not found")
	ErrStatusIncidentNotFound  = errors.New("store: status incident not found")
)

// NewStatusID mints an identifier with the given prefix.
func NewStatusID(prefix string) (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: generate status id: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

func statusTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseStatusTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}

func parseNullableStatusTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseStatusTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetStatusPageSettings returns the settings, or disabled defaults when never saved.
func (db *DB) GetStatusPageSettings(ctx context.Context) (StatusPageSettings, error) {
	var (
		s       StatusPageSettings
		enabled int
		updated string
	)
	err := db.QueryRowContext(ctx, `SELECT enabled, title, description, custom_domain, updated_at FROM status_page_settings WHERE id = 1`).
		Scan(&enabled, &s.Title, &s.Description, &s.CustomDomain, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusPageSettings{}, nil
	}
	if err != nil {
		return StatusPageSettings{}, fmt.Errorf("store: get status page settings: %w", err)
	}
	s.Enabled = enabled != 0
	if s.UpdatedAt, err = parseStatusTime(updated); err != nil {
		return StatusPageSettings{}, fmt.Errorf("store: status page settings: %w", err)
	}
	return s, nil
}

// SaveStatusPageSettings replaces the settings row.
func (db *DB) SaveStatusPageSettings(ctx context.Context, s StatusPageSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO status_page_settings (id, enabled, title, description, custom_domain, updated_at)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET enabled = excluded.enabled, title = excluded.title,
			description = excluded.description, custom_domain = excluded.custom_domain, updated_at = excluded.updated_at`,
		enabled, s.Title, s.Description, s.CustomDomain, statusTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save status page settings: %w", err)
	}
	return nil
}

const statusComponentColumns = `SELECT id, kind, target, display_name, position, created_at, updated_at FROM status_components`

func scanStatusComponent(scan func(dest ...any) error) (StatusComponent, error) {
	var (
		c       StatusComponent
		cr, upd string
	)
	if err := scan(&c.ID, &c.Kind, &c.Target, &c.DisplayName, &c.Position, &cr, &upd); err != nil {
		return StatusComponent{}, err
	}
	var err error
	if c.CreatedAt, err = parseStatusTime(cr); err != nil {
		return StatusComponent{}, err
	}
	if c.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return StatusComponent{}, err
	}
	return c, nil
}

// ListStatusComponents returns components in display order.
func (db *DB) ListStatusComponents(ctx context.Context) ([]StatusComponent, error) {
	rows, err := db.QueryContext(ctx, statusComponentColumns+` ORDER BY position, created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list status components: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusComponent
	for rows.Next() {
		c, err := scanStatusComponent(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan status component: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate status components: %w", err)
	}
	return out, nil
}

// GetStatusComponent returns one component or ErrStatusComponentNotFound.
func (db *DB) GetStatusComponent(ctx context.Context, id string) (StatusComponent, error) {
	c, err := scanStatusComponent(db.QueryRowContext(ctx, statusComponentColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusComponent{}, ErrStatusComponentNotFound
	}
	if err != nil {
		return StatusComponent{}, fmt.Errorf("store: get status component %q: %w", id, err)
	}
	return c, nil
}

// SaveStatusComponent creates or replaces a component.
func (db *DB) SaveStatusComponent(ctx context.Context, c StatusComponent) error {
	now := statusTime(time.Now())
	_, err := db.ExecContext(ctx, `
		INSERT INTO status_components (id, kind, target, display_name, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, target = excluded.target,
			display_name = excluded.display_name, position = excluded.position, updated_at = excluded.updated_at`,
		c.ID, c.Kind, c.Target, c.DisplayName, c.Position, now, now)
	if err != nil {
		return fmt.Errorf("store: save status component %q: %w", c.ID, err)
	}
	return nil
}

// DeleteStatusComponent removes a component and its counters.
func (db *DB) DeleteStatusComponent(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM status_components WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete status component %q: %w", id, err)
	}
	return nil
}

// AddStatusSamples adds probe counts to a component's counters for day (YYYY-MM-DD, UTC).
func (db *DB) AddStatusSamples(ctx context.Context, componentID, day string, ok, degraded, down int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO status_component_daily (component_id, day, ok_samples, degraded_samples, down_samples)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (component_id, day) DO UPDATE SET ok_samples = ok_samples + excluded.ok_samples,
			degraded_samples = degraded_samples + excluded.degraded_samples, down_samples = down_samples + excluded.down_samples`,
		componentID, day, ok, degraded, down)
	if err != nil {
		return fmt.Errorf("store: add status samples for %q: %w", componentID, err)
	}
	return nil
}

// ListStatusDaily returns counters for every component from sinceDay (inclusive), oldest first.
func (db *DB) ListStatusDaily(ctx context.Context, sinceDay string) ([]StatusDaily, error) {
	rows, err := db.QueryContext(ctx, `SELECT component_id, day, ok_samples, degraded_samples, down_samples
		FROM status_component_daily WHERE day >= ? ORDER BY day`, sinceDay)
	if err != nil {
		return nil, fmt.Errorf("store: list status daily: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusDaily
	for rows.Next() {
		var d StatusDaily
		if err := rows.Scan(&d.ComponentID, &d.Day, &d.OK, &d.Degraded, &d.Down); err != nil {
			return nil, fmt.Errorf("store: scan status daily: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate status daily: %w", err)
	}
	return out, nil
}

// PruneStatusDaily drops counters older than beforeDay.
func (db *DB) PruneStatusDaily(ctx context.Context, beforeDay string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM status_component_daily WHERE day < ?`, beforeDay); err != nil {
		return fmt.Errorf("store: prune status daily: %w", err)
	}
	return nil
}

const statusIncidentColumns = `SELECT id, kind, title, status, impact, component_ids, starts_at, ends_at, resolved_at, created_at, updated_at FROM status_incidents`

func scanStatusIncident(scan func(dest ...any) error) (StatusIncident, error) {
	var (
		i                      StatusIncident
		comps, starts, cr, upd string
		ends, resolved         sql.NullString
	)
	if err := scan(&i.ID, &i.Kind, &i.Title, &i.Status, &i.Impact, &comps, &starts, &ends, &resolved, &cr, &upd); err != nil {
		return StatusIncident{}, err
	}
	if err := json.Unmarshal([]byte(comps), &i.ComponentIDs); err != nil {
		return StatusIncident{}, fmt.Errorf("decode component ids: %w", err)
	}
	var err error
	if i.StartsAt, err = parseStatusTime(starts); err != nil {
		return StatusIncident{}, err
	}
	if i.EndsAt, err = parseNullableStatusTime(ends); err != nil {
		return StatusIncident{}, err
	}
	if i.ResolvedAt, err = parseNullableStatusTime(resolved); err != nil {
		return StatusIncident{}, err
	}
	if i.CreatedAt, err = parseStatusTime(cr); err != nil {
		return StatusIncident{}, err
	}
	if i.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return StatusIncident{}, err
	}
	return i, nil
}

func nullableStatusTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return statusTime(*t)
}

// SaveStatusIncident creates or replaces an incident.
func (db *DB) SaveStatusIncident(ctx context.Context, i StatusIncident) error {
	comps, err := json.Marshal(nonNilIDs(i.ComponentIDs))
	if err != nil {
		return fmt.Errorf("store: encode incident components: %w", err)
	}
	now := statusTime(time.Now())
	_, err = db.ExecContext(ctx, `
		INSERT INTO status_incidents (id, kind, title, status, impact, component_ids, starts_at, ends_at, resolved_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, title = excluded.title, status = excluded.status,
			impact = excluded.impact, component_ids = excluded.component_ids, starts_at = excluded.starts_at,
			ends_at = excluded.ends_at, resolved_at = excluded.resolved_at, updated_at = excluded.updated_at`,
		i.ID, i.Kind, i.Title, i.Status, i.Impact, string(comps), statusTime(i.StartsAt),
		nullableStatusTime(i.EndsAt), nullableStatusTime(i.ResolvedAt), now, now)
	if err != nil {
		return fmt.Errorf("store: save status incident %q: %w", i.ID, err)
	}
	return nil
}

func nonNilIDs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// GetStatusIncident returns one incident or ErrStatusIncidentNotFound.
func (db *DB) GetStatusIncident(ctx context.Context, id string) (StatusIncident, error) {
	i, err := scanStatusIncident(db.QueryRowContext(ctx, statusIncidentColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusIncident{}, ErrStatusIncidentNotFound
	}
	if err != nil {
		return StatusIncident{}, fmt.Errorf("store: get status incident %q: %w", id, err)
	}
	return i, nil
}

// ListStatusIncidents returns incidents newest first, capped at limit.
func (db *DB) ListStatusIncidents(ctx context.Context, limit int) ([]StatusIncident, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, statusIncidentColumns+` ORDER BY starts_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list status incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusIncident
	for rows.Next() {
		i, err := scanStatusIncident(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan status incident: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate status incidents: %w", err)
	}
	return out, nil
}

// DeleteStatusIncident removes an incident and its updates.
func (db *DB) DeleteStatusIncident(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM status_incidents WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete status incident %q: %w", id, err)
	}
	return nil
}

// AddStatusIncidentUpdate appends a timeline entry.
func (db *DB) AddStatusIncidentUpdate(ctx context.Context, u StatusIncidentUpdate) error {
	_, err := db.ExecContext(ctx, `INSERT INTO status_incident_updates (id, incident_id, status, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.IncidentID, u.Status, u.Body, statusTime(u.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: add status incident update: %w", err)
	}
	return nil
}

// ListStatusIncidentUpdates returns every update, oldest first.
func (db *DB) ListStatusIncidentUpdates(ctx context.Context) ([]StatusIncidentUpdate, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, incident_id, status, body, created_at FROM status_incident_updates ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list status incident updates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusIncidentUpdate
	for rows.Next() {
		var (
			u  StatusIncidentUpdate
			at string
		)
		if err := rows.Scan(&u.ID, &u.IncidentID, &u.Status, &u.Body, &at); err != nil {
			return nil, fmt.Errorf("store: scan status incident update: %w", err)
		}
		if u.CreatedAt, err = parseStatusTime(at); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate status incident updates: %w", err)
	}
	return out, nil
}
