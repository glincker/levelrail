package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Internal incident states (ADR 061), separate from the public status column.
const (
	IncidentStateTriggered    = "triggered"
	IncidentStateAcknowledged = "acknowledged"
	IncidentStateResolved     = "resolved"
)

// Update visibility values for status_incident_updates.
const (
	UpdateVisibilityPublic   = "public"
	UpdateVisibilityInternal = "internal"
)

// StatusIncidentInternal is the operator-only side of a status incident row.
type StatusIncidentInternal struct {
	Source             string
	SourceRef          string
	InternalState      string
	Published          bool
	PublishedAt        *time.Time
	PublishedBy        string
	AcknowledgedAt     *time.Time
	AcknowledgedBy     string
	InternalResolvedAt *time.Time
	DraftTitle         string
	DraftBody          string
}

// InternalStatusIncident pairs the public fields with the internal ones.
type InternalStatusIncident struct {
	StatusIncident
	StatusIncidentInternal
}

const internalIncidentColumns = `SELECT id, kind, title, status, impact, component_ids, starts_at, ends_at, resolved_at, created_at, updated_at,
	source, source_ref, internal_state, published, published_at, published_by, acknowledged_at, acknowledged_by,
	internal_resolved_at, draft_title, draft_body FROM status_incidents`

func scanInternalIncident(scan func(dest ...any) error) (InternalStatusIncident, error) {
	var (
		out                    InternalStatusIncident
		comps, starts, cr, upd string
		ends, resolved         sql.NullString
		ref, pubAt, pubBy      sql.NullString
		ackAt, ackBy, intRes   sql.NullString
		dTitle, dBody          sql.NullString
		published              int
	)
	i := &out.StatusIncident
	if err := scan(&i.ID, &i.Kind, &i.Title, &i.Status, &i.Impact, &comps, &starts, &ends, &resolved, &cr, &upd,
		&out.Source, &ref, &out.InternalState, &published, &pubAt, &pubBy, &ackAt, &ackBy, &intRes, &dTitle, &dBody); err != nil {
		return InternalStatusIncident{}, err
	}
	if err := json.Unmarshal([]byte(comps), &i.ComponentIDs); err != nil {
		return InternalStatusIncident{}, fmt.Errorf("decode component ids: %w", err)
	}
	var err error
	if i.StartsAt, err = parseStatusTime(starts); err != nil {
		return InternalStatusIncident{}, err
	}
	if i.EndsAt, err = parseNullableStatusTime(ends); err != nil {
		return InternalStatusIncident{}, err
	}
	if i.ResolvedAt, err = parseNullableStatusTime(resolved); err != nil {
		return InternalStatusIncident{}, err
	}
	if i.CreatedAt, err = parseStatusTime(cr); err != nil {
		return InternalStatusIncident{}, err
	}
	if i.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return InternalStatusIncident{}, err
	}
	out.SourceRef, out.PublishedBy = ref.String, pubBy.String
	out.AcknowledgedBy, out.DraftTitle, out.DraftBody = ackBy.String, dTitle.String, dBody.String
	out.Published = published == 1
	if out.PublishedAt, err = parseNullableStatusTime(pubAt); err != nil {
		return InternalStatusIncident{}, err
	}
	if out.AcknowledgedAt, err = parseNullableStatusTime(ackAt); err != nil {
		return InternalStatusIncident{}, err
	}
	if out.InternalResolvedAt, err = parseNullableStatusTime(intRes); err != nil {
		return InternalStatusIncident{}, err
	}
	return out, nil
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// CreateInternalStatusIncident inserts a system or promoted incident. The caller decides Published;
// system rows pass false so they stay off every public read until an operator publishes.
func (db *DB) CreateInternalStatusIncident(ctx context.Context, in InternalStatusIncident) error {
	comps, err := json.Marshal(nonNilIDs(in.ComponentIDs))
	if err != nil {
		return fmt.Errorf("store: encode incident components: %w", err)
	}
	pub := 0
	if in.Published {
		pub = 1
	}
	now := statusTime(time.Now())
	_, err = db.ExecContext(ctx, `
		INSERT INTO status_incidents (id, kind, title, status, impact, component_ids, starts_at, ends_at, resolved_at, created_at, updated_at,
			source, source_ref, internal_state, published, draft_title, draft_body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.Kind, in.Title, in.Status, in.Impact, string(comps), statusTime(in.StartsAt),
		nullableStatusTime(in.EndsAt), nullableStatusTime(in.ResolvedAt), now, now,
		in.Source, nullableText(in.SourceRef), in.InternalState, pub, nullableText(in.DraftTitle), nullableText(in.DraftBody))
	if err != nil {
		return fmt.Errorf("store: create internal status incident %q: %w", in.ID, err)
	}
	return nil
}

// GetInternalStatusIncident returns one incident with its internal fields, published or not.
func (db *DB) GetInternalStatusIncident(ctx context.Context, id string) (InternalStatusIncident, error) {
	i, err := scanInternalIncident(db.QueryRowContext(ctx, internalIncidentColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return InternalStatusIncident{}, ErrStatusIncidentNotFound
	}
	if err != nil {
		return InternalStatusIncident{}, fmt.Errorf("store: get internal status incident %q: %w", id, err)
	}
	return i, nil
}

// ListInternalStatusIncidents is the operator read: every row, newest first.
func (db *DB) ListInternalStatusIncidents(ctx context.Context, limit int) ([]InternalStatusIncident, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, internalIncidentColumns+` ORDER BY starts_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list internal status incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []InternalStatusIncident
	for rows.Next() {
		i, err := scanInternalIncident(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan internal status incident: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate internal status incidents: %w", err)
	}
	return out, nil
}

// SetStatusIncidentInternalState moves the internal lifecycle and stamps ack or resolve times.
func (db *DB) SetStatusIncidentInternalState(ctx context.Context, id, state, by string, at time.Time) error {
	ts, now := statusTime(at), statusTime(time.Now())
	var res sql.Result
	var err error
	switch state {
	case IncidentStateAcknowledged:
		res, err = db.ExecContext(ctx, `UPDATE status_incidents SET internal_state = ?, acknowledged_at = ?, acknowledged_by = ?, updated_at = ? WHERE id = ?`,
			state, ts, by, now, id)
	case IncidentStateResolved:
		res, err = db.ExecContext(ctx, `UPDATE status_incidents SET internal_state = ?, internal_resolved_at = ?, updated_at = ? WHERE id = ?`,
			state, ts, now, id)
	case IncidentStateTriggered:
		res, err = db.ExecContext(ctx, `UPDATE status_incidents SET internal_state = ?, internal_resolved_at = NULL, updated_at = ? WHERE id = ?`,
			state, now, id)
	default:
		return fmt.Errorf("store: unknown incident state %q", state)
	}
	if err != nil {
		return fmt.Errorf("store: set incident state %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStatusIncidentNotFound
	}
	return nil
}

// SaveStatusIncidentDraft stores the unpublished draft text; nothing public changes.
func (db *DB) SaveStatusIncidentDraft(ctx context.Context, id, title, body string) error {
	res, err := db.ExecContext(ctx, `UPDATE status_incidents SET draft_title = ?, draft_body = ?, updated_at = ? WHERE id = ?`,
		nullableText(title), nullableText(body), statusTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("store: save incident draft %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStatusIncidentNotFound
	}
	return nil
}

// AddInternalIncidentUpdate appends a timeline entry with an explicit visibility and author.
func (db *DB) AddInternalIncidentUpdate(ctx context.Context, u StatusIncidentUpdate, visibility, author string) error {
	if visibility != UpdateVisibilityPublic && visibility != UpdateVisibilityInternal {
		return fmt.Errorf("store: unknown update visibility %q", visibility)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO status_incident_updates (id, incident_id, status, body, created_at, visibility, author) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.IncidentID, u.Status, u.Body, statusTime(u.CreatedAt), visibility, nullableText(author))
	if err != nil {
		return fmt.Errorf("store: add internal incident update: %w", err)
	}
	return nil
}

// ListInternalIncidentUpdates returns the full timeline of one incident, internal entries included.
func (db *DB) ListInternalIncidentUpdates(ctx context.Context, incidentID string) ([]StatusIncidentUpdate, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, incident_id, status, body, created_at FROM status_incident_updates WHERE incident_id = ? ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("store: list internal incident updates: %w", err)
	}
	return scanIncidentUpdates(rows)
}

func scanIncidentUpdates(rows *sql.Rows) ([]StatusIncidentUpdate, error) {
	defer func() { _ = rows.Close() }()
	var out []StatusIncidentUpdate
	for rows.Next() {
		var (
			u  StatusIncidentUpdate
			at string
		)
		if err := rows.Scan(&u.ID, &u.IncidentID, &u.Status, &u.Body, &at); err != nil {
			return nil, fmt.Errorf("store: scan incident update: %w", err)
		}
		var err error
		if u.CreatedAt, err = parseStatusTime(at); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate incident updates: %w", err)
	}
	return out, nil
}

// ListPublicStatusIncidents is the only incident read a public surface may use: published rows only.
func (db *DB) ListPublicStatusIncidents(ctx context.Context, limit int) ([]StatusIncident, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, statusIncidentColumns+` WHERE published = 1 ORDER BY starts_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list public status incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusIncident
	for rows.Next() {
		i, err := scanStatusIncident(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan public status incident: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate public status incidents: %w", err)
	}
	return out, nil
}

// GetPublicStatusIncident returns a published incident or ErrStatusIncidentNotFound.
func (db *DB) GetPublicStatusIncident(ctx context.Context, id string) (StatusIncident, error) {
	i, err := scanStatusIncident(db.QueryRowContext(ctx, statusIncidentColumns+` WHERE id = ? AND published = 1`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusIncident{}, ErrStatusIncidentNotFound
	}
	if err != nil {
		return StatusIncident{}, fmt.Errorf("store: get public status incident %q: %w", id, err)
	}
	return i, nil
}

// ListPublicStatusIncidentUpdates returns public entries of published incidents, oldest first.
func (db *DB) ListPublicStatusIncidentUpdates(ctx context.Context) ([]StatusIncidentUpdate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.incident_id, u.status, u.body, u.created_at
		FROM status_incident_updates u JOIN status_incidents i ON i.id = u.incident_id
		WHERE u.visibility = 'public' AND i.published = 1 ORDER BY u.created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list public status incident updates: %w", err)
	}
	return scanIncidentUpdates(rows)
}

// AddStatusIncidentLink records a monitor going down under an incident.
func (db *DB) AddStatusIncidentLink(ctx context.Context, incidentID, monitorID string, downAt time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO status_incident_links (incident_id, monitor_id, down_at) VALUES (?, ?, ?)`,
		incidentID, monitorID, statusTime(downAt))
	if err != nil {
		return fmt.Errorf("store: add incident link: %w", err)
	}
	return nil
}

// MarkStatusIncidentLinkUp stamps recovery on the open link of a monitor.
func (db *DB) MarkStatusIncidentLinkUp(ctx context.Context, incidentID, monitorID string, upAt time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE status_incident_links SET up_at = ? WHERE incident_id = ? AND monitor_id = ? AND up_at IS NULL`,
		statusTime(upAt), incidentID, monitorID)
	if err != nil {
		return fmt.Errorf("store: mark incident link up: %w", err)
	}
	return nil
}
