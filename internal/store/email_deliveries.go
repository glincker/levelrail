package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Email delivery statuses stored in email_deliveries.status.
const (
	EmailDeliverySent    = "sent"
	EmailDeliveryFailed  = "failed"
	EmailDeliveryBounced = "bounced"
)

// emailDeliveryTimeFormat is fixed-width so created_at sorts and compares as text.
const emailDeliveryTimeFormat = "2006-01-02T15:04:05.000000000Z"

// EmailDelivery is one logged send outcome.
type EmailDelivery struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"`
	ToHash    string    `json:"to_hash"`
	ToDisplay string    `json:"to"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	MessageID string    `json:"message_id"`
	Error     string    `json:"error"`
	ErrorKind string    `json:"error_kind"`
	Attempts  int       `json:"attempts"`

	// ProviderID and FailoverFrom are internal until the API slice exposes them.
	ProviderID   string `json:"-"`
	FailoverFrom string `json:"-"`
}

// EmailDeliveryFilter narrows ListEmailDeliveries.
type EmailDeliveryFilter struct {
	Status   string
	Provider string
	Kind     string
	// Query matches the message id, the error text, or a recipient (by hash, or by substring of the stored display).
	Query  string
	Since  time.Time
	Before time.Time
	Limit  int
}

// EmailRecipientHash is the stable lookup key stored instead of the address.
func EmailRecipientHash(addr string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(addr))))
	return hex.EncodeToString(sum[:8])
}

// EmailRecipientDisplay truncates an address to "a***@domain" unless full is set.
func EmailRecipientDisplay(addr string, full bool) string {
	addr = strings.TrimSpace(addr)
	if full {
		return addr
	}
	at := strings.LastIndex(addr, "@")
	if at <= 0 {
		return "***"
	}
	return addr[:1] + "***" + addr[at:]
}

// InsertEmailDelivery appends one delivery row; the recipient is hashed and, unless fullAddress, truncated.
func (db *DB) InsertEmailDelivery(ctx context.Context, d EmailDelivery, rawTo string, fullAddress bool) error {
	if d.ID == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("store: email delivery id: %w", err)
		}
		d.ID = "ed-" + hex.EncodeToString(b)
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	if d.Attempts < 1 {
		d.Attempts = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO email_deliveries (id, created_at, provider, to_hash, to_display, kind, status, message_id, error, error_kind, attempts, provider_id, failover_from)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.CreatedAt.UTC().Format(emailDeliveryTimeFormat), d.Provider, EmailRecipientHash(rawTo), EmailRecipientDisplay(rawTo, fullAddress),
		d.Kind, d.Status, d.MessageID, d.Error, d.ErrorKind, d.Attempts, d.ProviderID, d.FailoverFrom)
	if err != nil {
		return fmt.Errorf("store: insert email delivery: %w", err)
	}
	return nil
}

const emailDeliveryColumns = `id, created_at, provider, to_hash, to_display, kind, status, message_id, error, error_kind, attempts, provider_id, failover_from`

func scanEmailDelivery(sc interface{ Scan(...any) error }) (EmailDelivery, error) {
	var d EmailDelivery
	var created string
	if err := sc.Scan(&d.ID, &created, &d.Provider, &d.ToHash, &d.ToDisplay, &d.Kind, &d.Status, &d.MessageID, &d.Error, &d.ErrorKind, &d.Attempts, &d.ProviderID, &d.FailoverFrom); err != nil {
		return EmailDelivery{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return EmailDelivery{}, fmt.Errorf("parse created_at: %w", err)
	}
	d.CreatedAt = t
	return d, nil
}

// ListEmailDeliveries returns deliveries newest first.
func (db *DB) ListEmailDeliveries(ctx context.Context, f EmailDeliveryFilter) ([]EmailDelivery, error) {
	q := `SELECT ` + emailDeliveryColumns + ` FROM email_deliveries WHERE 1=1`
	var args []any
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.Provider != "" {
		q += ` AND provider = ?`
		args = append(args, f.Provider)
	}
	if f.Kind != "" {
		q += ` AND kind = ?`
		args = append(args, f.Kind)
	}
	if !f.Since.IsZero() {
		q += ` AND created_at >= ?`
		args = append(args, f.Since.UTC().Format(emailDeliveryTimeFormat))
	}
	if !f.Before.IsZero() {
		q += ` AND created_at < ?`
		args = append(args, f.Before.UTC().Format(emailDeliveryTimeFormat))
	}
	if f.Query != "" {
		like := "%" + strings.ToLower(f.Query) + "%"
		q += ` AND (to_hash = ? OR lower(to_display) LIKE ? OR lower(message_id) LIKE ? OR lower(error) LIKE ?)`
		args = append(args, EmailRecipientHash(f.Query), like, like, like)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list email deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EmailDelivery
	for rows.Next() {
		d, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan email delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetEmailDelivery returns one delivery by id, or sql.ErrNoRows.
func (db *DB) GetEmailDelivery(ctx context.Context, id string) (EmailDelivery, error) {
	d, err := scanEmailDelivery(db.QueryRowContext(ctx, `SELECT `+emailDeliveryColumns+` FROM email_deliveries WHERE id = ?`, id))
	if err != nil {
		return EmailDelivery{}, fmt.Errorf("store: get email delivery: %w", err)
	}
	return d, nil
}

// CountEmailDeliveriesSent counts successful sends at or after since.
func (db *DB) CountEmailDeliveriesSent(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_deliveries WHERE status = 'sent' AND created_at >= ?`,
		since.UTC().Format(emailDeliveryTimeFormat)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count email deliveries: %w", err)
	}
	return n, nil
}

// EmailDeliveryStats summarizes the delivery log since a point in time.
type EmailDeliveryStats struct {
	Sent        int
	Failed      int
	LastSuccess *EmailDelivery
	LastFailure *EmailDelivery
}

// EmailDeliveryStats counts sends and failures since the given time and finds the latest of each overall.
func (db *DB) EmailDeliveryStats(ctx context.Context, since time.Time) (EmailDeliveryStats, error) {
	var st EmailDeliveryStats
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(status = 'sent'), 0), COALESCE(SUM(status = 'failed'), 0)
		FROM email_deliveries WHERE created_at >= ?`, since.UTC().Format(emailDeliveryTimeFormat)).Scan(&st.Sent, &st.Failed)
	if err != nil {
		return st, fmt.Errorf("store: email delivery stats: %w", err)
	}
	for _, p := range []struct {
		status string
		dst    **EmailDelivery
	}{{EmailDeliverySent, &st.LastSuccess}, {EmailDeliveryFailed, &st.LastFailure}} {
		d, err := scanEmailDelivery(db.QueryRowContext(ctx, `SELECT `+emailDeliveryColumns+` FROM email_deliveries WHERE status = ? ORDER BY created_at DESC LIMIT 1`, p.status))
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return st, fmt.Errorf("store: email delivery stats: %w", err)
		default:
			*p.dst = &d
		}
	}
	return st, nil
}

// PruneEmailDeliveries deletes rows older than before and returns how many went.
func (db *DB) PruneEmailDeliveries(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM email_deliveries WHERE created_at < ?`, before.UTC().Format(emailDeliveryTimeFormat))
	if err != nil {
		return 0, fmt.Errorf("store: prune email deliveries: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
