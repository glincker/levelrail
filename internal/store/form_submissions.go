package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const submissionColumns = `id, form_id, fields, spam, spam_reason, ip_hash, created_at, test`

func scanSubmission(scan func(dest ...any) error) (FormSubmission, error) {
	var s FormSubmission
	var fields, created string
	var spam, test int
	if err := scan(&s.ID, &s.FormID, &fields, &spam, &s.SpamReason, &s.IPHash, &created, &test); err != nil {
		return FormSubmission{}, err
	}
	if err := json.Unmarshal([]byte(fields), &s.Fields); err != nil {
		return FormSubmission{}, fmt.Errorf("fields: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return FormSubmission{}, fmt.Errorf("created_at: %w", err)
	}
	s.Spam, s.Test, s.CreatedAt = spam != 0, test != 0, t
	return s, nil
}

// AddFormSubmission stores s and trims the form to maxStored rows (spam first, then oldest);
// maxStored <= 0 disables the cap. It returns the stored row with its id.
func (db *DB) AddFormSubmission(ctx context.Context, s FormSubmission, maxStored int) (FormSubmission, error) {
	fields, err := json.Marshal(s.Fields)
	if err != nil {
		return FormSubmission{}, fmt.Errorf("store: add form submission: %w", err)
	}
	spam := 0
	if s.Spam {
		spam = 1
	}
	test := 0
	if s.Test {
		test = 1
	}
	res, err := db.ExecContext(ctx, `INSERT INTO form_submissions (form_id, fields, spam, spam_reason, ip_hash, created_at, test) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.FormID, string(fields), spam, s.SpamReason, s.IPHash, s.CreatedAt.UTC().Format(time.RFC3339Nano), test)
	if err != nil {
		return FormSubmission{}, fmt.Errorf("store: add form submission: %w", err)
	}
	if s.ID, err = res.LastInsertId(); err != nil {
		return FormSubmission{}, fmt.Errorf("store: add form submission: %w", err)
	}
	if maxStored > 0 {
		// Keep ham over spam, newest over oldest: rank by (spam asc, id desc) and drop the tail.
		if _, err := db.ExecContext(ctx, `DELETE FROM form_submissions WHERE id IN (
			SELECT id FROM form_submissions WHERE form_id = ? ORDER BY spam ASC, id DESC LIMIT -1 OFFSET ?)`, s.FormID, maxStored); err != nil {
			return s, fmt.Errorf("store: trim form submissions: %w", err)
		}
	}
	return s, nil
}

// ListFormSubmissions returns submissions newest first, filtered by q.
func (db *DB) ListFormSubmissions(ctx context.Context, formID string, q FormSubmissionQuery) ([]FormSubmission, error) {
	where := []string{"form_id = ?"}
	args := []any{formID}
	if q.Spam != nil {
		where = append(where, "spam = ?")
		if *q.Spam {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if q.BeforeID > 0 {
		where = append(where, "id < ?")
		args = append(args, q.BeforeID)
	}
	if term := strings.TrimSpace(q.Search); term != "" {
		where = append(where, `fields LIKE ? ESCAPE '\'`)
		args = append(args, "%"+likeEscape(term)+"%")
	}
	query := `SELECT ` + submissionColumns + ` FROM form_submissions WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list form submissions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []FormSubmission
	for rows.Next() {
		s, err := scanSubmission(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list form submissions: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetFormSubmissionSpam flags or clears one submission of formID.
func (db *DB) SetFormSubmissionSpam(ctx context.Context, formID string, id int64, spam bool, reason string) error {
	flag := 0
	if spam {
		flag = 1
	}
	res, err := db.ExecContext(ctx, `UPDATE form_submissions SET spam = ?, spam_reason = ? WHERE id = ? AND form_id = ?`, flag, reason, id, formID)
	if err != nil {
		return fmt.Errorf("store: set form submission spam: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFormSubmissionNotFound
	}
	return nil
}

// DeleteFormSubmission removes one submission of formID.
func (db *DB) DeleteFormSubmission(ctx context.Context, formID string, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM form_submissions WHERE id = ? AND form_id = ?`, id, formID)
	if err != nil {
		return fmt.Errorf("store: delete form submission: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFormSubmissionNotFound
	}
	return nil
}

// PruneFormSubmissions deletes submissions older than cutoff across all forms.
func (db *DB) PruneFormSubmissions(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM form_submissions WHERE created_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: prune form submissions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
