package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrMailNotFound is returned when a mail template, group or broadcast does not exist.
var ErrMailNotFound = errors.New("store: mail record not found")

// MailTemplateVersionsKept is how many saved versions each template keeps.
const MailTemplateVersionsKept = 10

// Broadcast statuses.
const (
	MailBroadcastScheduled = "scheduled"
	MailBroadcastSending   = "sending"
	MailBroadcastCompleted = "completed"
	MailBroadcastCancelled = "cancelled"
	MailBroadcastFailed    = "failed"
)

// Broadcast recipient statuses.
const (
	MailRecipientPending    = "pending"
	MailRecipientSent       = "sent"
	MailRecipientFailed     = "failed"
	MailRecipientSuppressed = "suppressed"
	MailRecipientCancelled  = "cancelled"
)

// MailTemplate is an operator override of one event template.
type MailTemplate struct {
	EventKey  string `json:"event_key"`
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
}

// MailTemplateVersion is one saved revision of a template override.
type MailTemplateVersion struct {
	EventKey  string `json:"event_key"`
	Version   int    `json:"version"`
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// MailLayout is the single shared email frame setting.
type MailLayout struct {
	HeaderImageURL string `json:"header_image_url"`
	FooterText     string `json:"footer_text"`
}

// MailGroup is a saved audience. Rules is opaque JSON owned by internal/mailgroups.
type MailGroup struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Rules       json.RawMessage `json:"rules"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// MailSuppression is an address that opted out of announcements.
type MailSuppression struct {
	Email     string `json:"email"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// MailBroadcast is one composed message and its sending progress.
type MailBroadcast struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Body        string   `json:"body"`
	TemplateKey string   `json:"template_key"`
	GroupID     string   `json:"group_id"`
	UserIDs     []string `json:"user_ids"`
	Status      string   `json:"status"`
	ScheduledAt string   `json:"scheduled_at"`
	Total       int      `json:"total"`
	Sent        int      `json:"sent"`
	Failed      int      `json:"failed"`
	Suppressed  int      `json:"suppressed"`
	Error       string   `json:"error"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	StartedAt   string   `json:"started_at"`
	FinishedAt  string   `json:"finished_at"`
}

// MailRecipient is one address of a broadcast and what happened to it.
type MailRecipient struct {
	BroadcastID string `json:"broadcast_id"`
	Email       string `json:"email"`
	UserID      string `json:"user_id"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	SentAt      string `json:"sent_at"`
}

func mailNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// ListMailTemplates returns every stored override.
func (db *DB) ListMailTemplates(ctx context.Context) ([]MailTemplate, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_key, subject, preheader, body, updated_by, updated_at FROM mail_templates ORDER BY event_key`)
	if err != nil {
		return nil, fmt.Errorf("store: list mail templates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailTemplate
	for rows.Next() {
		var t MailTemplate
		if err := rows.Scan(&t.EventKey, &t.Subject, &t.Preheader, &t.Body, &t.UpdatedBy, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan mail template: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetMailTemplate returns the override for key, or ErrMailNotFound.
func (db *DB) GetMailTemplate(ctx context.Context, key string) (*MailTemplate, error) {
	var t MailTemplate
	err := db.QueryRowContext(ctx, `SELECT event_key, subject, preheader, body, updated_by, updated_at FROM mail_templates WHERE event_key = ?`, key).
		Scan(&t.EventKey, &t.Subject, &t.Preheader, &t.Body, &t.UpdatedBy, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMailNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get mail template %q: %w", key, err)
	}
	return &t, nil
}

// SaveMailTemplate writes the override and records a version, keeping the newest MailTemplateVersionsKept.
func (db *DB) SaveMailTemplate(ctx context.Context, t MailTemplate) error {
	now := mailNow()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save mail template: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_templates (event_key, subject, preheader, body, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_key) DO UPDATE SET subject = excluded.subject, preheader = excluded.preheader, body = excluded.body, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		t.EventKey, t.Subject, t.Preheader, t.Body, t.UpdatedBy, now); err != nil {
		return fmt.Errorf("store: save mail template %q: %w", t.EventKey, err)
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM mail_template_versions WHERE event_key = ?`, t.EventKey).Scan(&next); err != nil {
		return fmt.Errorf("store: next mail template version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_template_versions (event_key, version, subject, preheader, body, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.EventKey, next, t.Subject, t.Preheader, t.Body, t.UpdatedBy, now); err != nil {
		return fmt.Errorf("store: save mail template version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mail_template_versions WHERE event_key = ? AND version <= ?`, t.EventKey, next-MailTemplateVersionsKept); err != nil {
		return fmt.Errorf("store: prune mail template versions: %w", err)
	}
	return tx.Commit()
}

// DeleteMailTemplate removes the override so the built-in default applies again. History is kept.
func (db *DB) DeleteMailTemplate(ctx context.Context, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM mail_templates WHERE event_key = ?`, key); err != nil {
		return fmt.Errorf("store: reset mail template %q: %w", key, err)
	}
	return nil
}

// ListMailTemplateVersions returns key's saved versions, newest first.
func (db *DB) ListMailTemplateVersions(ctx context.Context, key string) ([]MailTemplateVersion, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_key, version, subject, preheader, body, created_by, created_at FROM mail_template_versions WHERE event_key = ? ORDER BY version DESC`, key)
	if err != nil {
		return nil, fmt.Errorf("store: list mail template versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailTemplateVersion
	for rows.Next() {
		var v MailTemplateVersion
		if err := rows.Scan(&v.EventKey, &v.Version, &v.Subject, &v.Preheader, &v.Body, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan mail template version: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetMailTemplateVersion returns one saved version, or ErrMailNotFound.
func (db *DB) GetMailTemplateVersion(ctx context.Context, key string, version int) (*MailTemplateVersion, error) {
	var v MailTemplateVersion
	err := db.QueryRowContext(ctx, `SELECT event_key, version, subject, preheader, body, created_by, created_at FROM mail_template_versions WHERE event_key = ? AND version = ?`, key, version).
		Scan(&v.EventKey, &v.Version, &v.Subject, &v.Preheader, &v.Body, &v.CreatedBy, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMailNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get mail template version: %w", err)
	}
	return &v, nil
}

// GetMailLayout returns the shared layout setting.
func (db *DB) GetMailLayout(ctx context.Context) (MailLayout, error) {
	var l MailLayout
	if err := db.QueryRowContext(ctx, `SELECT header_image_url, footer_text FROM mail_layout WHERE id = 1`).Scan(&l.HeaderImageURL, &l.FooterText); err != nil {
		return MailLayout{}, fmt.Errorf("store: get mail layout: %w", err)
	}
	return l, nil
}

// SetMailLayout replaces the shared layout setting.
func (db *DB) SetMailLayout(ctx context.Context, l MailLayout) error {
	if _, err := db.ExecContext(ctx, `UPDATE mail_layout SET header_image_url = ?, footer_text = ? WHERE id = 1`, l.HeaderImageURL, l.FooterText); err != nil {
		return fmt.Errorf("store: set mail layout: %w", err)
	}
	return nil
}

// ListMailGroups returns every group by name.
func (db *DB) ListMailGroups(ctx context.Context) ([]MailGroup, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, description, rules, created_by, created_at, updated_at FROM mail_groups ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list mail groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailGroup
	for rows.Next() {
		g, err := scanMailGroup(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func scanMailGroup(scan func(...any) error) (*MailGroup, error) {
	var g MailGroup
	var rules string
	if err := scan(&g.ID, &g.Name, &g.Description, &rules, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, fmt.Errorf("store: scan mail group: %w", err)
	}
	g.Rules = json.RawMessage(rules)
	return &g, nil
}

// GetMailGroup returns one group by id or name, or ErrMailNotFound.
func (db *DB) GetMailGroup(ctx context.Context, idOrName string) (*MailGroup, error) {
	g, err := scanMailGroup(db.QueryRowContext(ctx, `SELECT id, name, description, rules, created_by, created_at, updated_at FROM mail_groups WHERE id = ? OR name = ?`, idOrName, idOrName).Scan)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMailNotFound
	}
	return g, err
}

// SaveMailGroup inserts or updates a group.
func (db *DB) SaveMailGroup(ctx context.Context, g MailGroup) error {
	now := mailNow()
	if g.CreatedAt == "" {
		g.CreatedAt = now
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO mail_groups (id, name, description, rules, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, description = excluded.description, rules = excluded.rules, updated_at = excluded.updated_at`,
		g.ID, g.Name, g.Description, string(g.Rules), g.CreatedBy, g.CreatedAt, now); err != nil {
		return fmt.Errorf("store: save mail group %q: %w", g.ID, err)
	}
	return nil
}

// DeleteMailGroup removes a group by id.
func (db *DB) DeleteMailGroup(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM mail_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete mail group %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMailNotFound
	}
	return nil
}

// ListMailSuppressions returns every suppressed address.
func (db *DB) ListMailSuppressions(ctx context.Context) ([]MailSuppression, error) {
	rows, err := db.QueryContext(ctx, `SELECT email, reason, created_at FROM mail_suppressions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list mail suppressions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailSuppression
	for rows.Next() {
		var s MailSuppression
		if err := rows.Scan(&s.Email, &s.Reason, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan mail suppression: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddMailSuppression records an opt-out; an address already suppressed keeps its first record.
func (db *DB) AddMailSuppression(ctx context.Context, email, reason string) error {
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO mail_suppressions (email, reason, created_at) VALUES (?, ?, ?)`, strings.ToLower(email), reason, mailNow()); err != nil {
		return fmt.Errorf("store: add mail suppression: %w", err)
	}
	return nil
}

// RemoveMailSuppression lifts an opt-out.
func (db *DB) RemoveMailSuppression(ctx context.Context, email string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM mail_suppressions WHERE email = ?`, strings.ToLower(email)); err != nil {
		return fmt.Errorf("store: remove mail suppression: %w", err)
	}
	return nil
}

const mailBroadcastColumns = `id, subject, body, template_key, group_id, user_ids, status, scheduled_at, total, sent, failed, suppressed, error, created_by, created_at, started_at, finished_at`

func scanMailBroadcast(scan func(...any) error) (*MailBroadcast, error) {
	var b MailBroadcast
	var users string
	if err := scan(&b.ID, &b.Subject, &b.Body, &b.TemplateKey, &b.GroupID, &users, &b.Status, &b.ScheduledAt, &b.Total, &b.Sent, &b.Failed, &b.Suppressed, &b.Error, &b.CreatedBy, &b.CreatedAt, &b.StartedAt, &b.FinishedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(users), &b.UserIDs); err != nil {
		return nil, fmt.Errorf("store: decode broadcast users: %w", err)
	}
	return &b, nil
}

// CreateMailBroadcast inserts a broadcast with its recipients in one transaction.
func (db *DB) CreateMailBroadcast(ctx context.Context, b MailBroadcast, recipients []MailRecipient) error {
	users, err := json.Marshal(nonNilSlice(b.UserIDs))
	if err != nil {
		return fmt.Errorf("store: encode broadcast users: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create broadcast: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO mail_broadcasts (`+mailBroadcastColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Subject, b.Body, b.TemplateKey, b.GroupID, string(users), b.Status, b.ScheduledAt, b.Total, b.Sent, b.Failed, b.Suppressed, b.Error, b.CreatedBy, b.CreatedAt, b.StartedAt, b.FinishedAt); err != nil {
		return fmt.Errorf("store: insert broadcast: %w", err)
	}
	for _, r := range recipients {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mail_broadcast_recipients (broadcast_id, email, user_id, status) VALUES (?, ?, ?, ?)`, b.ID, r.Email, r.UserID, r.Status); err != nil {
			return fmt.Errorf("store: insert broadcast recipient: %w", err)
		}
	}
	return tx.Commit()
}

// GetMailBroadcast returns one broadcast, or ErrMailNotFound.
func (db *DB) GetMailBroadcast(ctx context.Context, id string) (*MailBroadcast, error) {
	b, err := scanMailBroadcast(db.QueryRowContext(ctx, `SELECT `+mailBroadcastColumns+` FROM mail_broadcasts WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMailNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get broadcast: %w", err)
	}
	return b, nil
}

// ListMailBroadcasts returns the newest broadcasts first.
func (db *DB) ListMailBroadcasts(ctx context.Context, limit int) ([]MailBroadcast, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+mailBroadcastColumns+` FROM mail_broadcasts ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list broadcasts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailBroadcast
	for rows.Next() {
		b, err := scanMailBroadcast(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan broadcast: %w", err)
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// ListMailBroadcastsByStatus returns broadcasts in any of statuses.
func (db *DB) ListMailBroadcastsByStatus(ctx context.Context, statuses ...string) ([]MailBroadcast, error) {
	all, err := db.ListMailBroadcasts(ctx, 500)
	if err != nil {
		return nil, err
	}
	var out []MailBroadcast
	for _, b := range all {
		for _, s := range statuses {
			if b.Status == s {
				out = append(out, b)
			}
		}
	}
	return out, nil
}

// CountMailBroadcastRecipientsSince counts recipients of broadcasts created at or after since, for the daily cap.
func (db *DB) CountMailBroadcastRecipientsSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(total - suppressed), 0) FROM mail_broadcasts WHERE created_at >= ? AND status != ?`, since.UTC().Format(time.RFC3339Nano), MailBroadcastCancelled).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count broadcast recipients: %w", err)
	}
	return n, nil
}

// UpdateMailBroadcast writes the mutable progress fields of b.
func (db *DB) UpdateMailBroadcast(ctx context.Context, b MailBroadcast) error {
	if _, err := db.ExecContext(ctx, `UPDATE mail_broadcasts SET status = ?, scheduled_at = ?, sent = ?, failed = ?, error = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		b.Status, b.ScheduledAt, b.Sent, b.Failed, b.Error, b.StartedAt, b.FinishedAt, b.ID); err != nil {
		return fmt.Errorf("store: update broadcast %q: %w", b.ID, err)
	}
	return nil
}

// ListMailRecipients returns a broadcast's recipients, optionally only in status.
func (db *DB) ListMailRecipients(ctx context.Context, broadcastID, status string, limit int) ([]MailRecipient, error) {
	q := `SELECT broadcast_id, email, user_id, status, error, sent_at FROM mail_broadcast_recipients WHERE broadcast_id = ?`
	args := []any{broadcastID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY email LIMIT ?`
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list broadcast recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MailRecipient
	for rows.Next() {
		var r MailRecipient
		if err := rows.Scan(&r.BroadcastID, &r.Email, &r.UserID, &r.Status, &r.Error, &r.SentAt); err != nil {
			return nil, fmt.Errorf("store: scan broadcast recipient: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetMailRecipientStatus records the outcome for one recipient.
func (db *DB) SetMailRecipientStatus(ctx context.Context, broadcastID, email, status, errText string) error {
	sentAt := ""
	if status == MailRecipientSent {
		sentAt = mailNow()
	}
	if _, err := db.ExecContext(ctx, `UPDATE mail_broadcast_recipients SET status = ?, error = ?, sent_at = ? WHERE broadcast_id = ? AND email = ?`, status, errText, sentAt, broadcastID, email); err != nil {
		return fmt.Errorf("store: set recipient status: %w", err)
	}
	return nil
}

// CancelPendingMailRecipients marks a broadcast's unsent recipients cancelled.
func (db *DB) CancelPendingMailRecipients(ctx context.Context, broadcastID string) error {
	if _, err := db.ExecContext(ctx, `UPDATE mail_broadcast_recipients SET status = ? WHERE broadcast_id = ? AND status = ?`, MailRecipientCancelled, broadcastID, MailRecipientPending); err != nil {
		return fmt.Errorf("store: cancel pending recipients: %w", err)
	}
	return nil
}

// UpdateMailBroadcastProgress writes only the counters, so a concurrent cancel is never overwritten.
func (db *DB) UpdateMailBroadcastProgress(ctx context.Context, id string, sent, failed, suppressed int) error {
	if _, err := db.ExecContext(ctx, `UPDATE mail_broadcasts SET sent = ?, failed = ?, suppressed = ? WHERE id = ?`, sent, failed, suppressed, id); err != nil {
		return fmt.Errorf("store: update broadcast progress %q: %w", id, err)
	}
	return nil
}

// FinishMailBroadcast sets the final status, but only while the broadcast is still sending.
func (db *DB) FinishMailBroadcast(ctx context.Context, b MailBroadcast) error {
	if _, err := db.ExecContext(ctx, `UPDATE mail_broadcasts SET status = ?, sent = ?, failed = ?, suppressed = ?, error = ?, finished_at = ? WHERE id = ? AND status = ?`,
		b.Status, b.Sent, b.Failed, b.Suppressed, b.Error, b.FinishedAt, b.ID, MailBroadcastSending); err != nil {
		return fmt.Errorf("store: finish broadcast %q: %w", b.ID, err)
	}
	return nil
}
