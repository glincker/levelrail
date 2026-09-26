package supplychain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const recordColumns = `attempt_id, app_name, sbom_format, package_count, sbom_bytes, has_provenance, summary, generated_at,
	scan_status, scanner, scan_error, scanned_at, vuln_counts, top_vulns, gate_action, gate_reason`

// SQLStore implements Store over the control plane database.
type SQLStore struct{ db *store.DB }

// NewSQLStore wraps db.
func NewSQLStore(db *store.DB) *SQLStore { return &SQLStore{db: db} }

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetSettings returns the stored settings, or the default (off) when none exist.
func (s *SQLStore) GetSettings(ctx context.Context, app string) (Settings, error) {
	out := Settings{App: app, Gate: GateOff}
	var gate, armed string
	err := s.db.QueryRowContext(ctx, `SELECT scan_enabled, scan_gate, override_reason, override_armed_at FROM app_supply_chain_settings WHERE app_name = ?`, app).
		Scan(&out.Enabled, &gate, &out.OverrideReason, &armed)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("store: get supply chain settings for %q: %w", app, err)
	}
	if mode, perr := ParseGateMode(gate); perr == nil {
		out.Gate = mode
	}
	out.OverrideArmedAt = parseTime(armed)
	return out, nil
}

// SaveSettings upserts settings.
func (s *SQLStore) SaveSettings(ctx context.Context, st Settings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_supply_chain_settings (app_name, scan_enabled, scan_gate, override_reason, override_armed_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET scan_enabled = excluded.scan_enabled, scan_gate = excluded.scan_gate,
			override_reason = excluded.override_reason, override_armed_at = excluded.override_armed_at, updated_at = excluded.updated_at
	`, st.App, st.Enabled, string(st.Gate), st.OverrideReason, fmtTime(st.OverrideArmedAt), fmtTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save supply chain settings for %q: %w", st.App, err)
	}
	return nil
}

// ConsumeOverride returns and clears an armed override newer than notBefore.
func (s *SQLStore) ConsumeOverride(ctx context.Context, app string, notBefore time.Time) (string, bool, error) {
	cur, err := s.GetSettings(ctx, app)
	if err != nil {
		return "", false, err
	}
	if cur.OverrideReason == "" || cur.OverrideArmedAt.IsZero() {
		return "", false, nil
	}
	res, err := s.db.ExecContext(ctx, `UPDATE app_supply_chain_settings SET override_reason = '', override_armed_at = '' WHERE app_name = ? AND override_armed_at = ?`, app, fmtTime(cur.OverrideArmedAt))
	if err != nil {
		return "", false, fmt.Errorf("store: consume supply chain override for %q: %w", app, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", false, nil
	}
	if cur.OverrideArmedAt.Before(notBefore) {
		return "", false, nil
	}
	return cur.OverrideReason, true, nil
}

// SaveRecord upserts a record.
func (s *SQLStore) SaveRecord(ctx context.Context, r Record) error {
	summary, err := json.Marshal(r.Summary)
	if err != nil {
		return fmt.Errorf("store: marshal supply chain summary: %w", err)
	}
	var counts, top []byte
	if r.Scan != nil {
		if counts, err = json.Marshal(r.Scan.Counts); err != nil {
			return fmt.Errorf("store: marshal vuln counts: %w", err)
		}
		if top, err = json.Marshal(scanDetail{Fixable: r.Scan.Fixable, Top: r.Scan.Top}); err != nil {
			return fmt.Errorf("store: marshal top vulns: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO deploy_supply_chain (`+recordColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(attempt_id) DO UPDATE SET sbom_format = excluded.sbom_format, package_count = excluded.package_count,
			sbom_bytes = excluded.sbom_bytes, has_provenance = excluded.has_provenance, summary = excluded.summary,
			scan_status = excluded.scan_status, scanner = excluded.scanner, scan_error = excluded.scan_error, scanned_at = excluded.scanned_at,
			vuln_counts = excluded.vuln_counts, top_vulns = excluded.top_vulns, gate_action = excluded.gate_action, gate_reason = excluded.gate_reason
	`, r.AttemptID, r.App, r.Summary.Format, r.Summary.PackageCount, r.SBOMBytes, r.HasProvenance, string(summary), fmtTime(r.GeneratedAt),
		r.ScanStatus, r.Scanner, r.ScanError, fmtTime(r.ScannedAt), string(counts), string(top), r.GateAction, r.GateReason)
	if err != nil {
		return fmt.Errorf("store: save supply chain record %q: %w", r.AttemptID, err)
	}
	return nil
}

type scanDetail struct {
	Fixable int    `json:"fixable"`
	Top     []Vuln `json:"top,omitempty"`
}

func scanRecord(scan func(dest ...any) error) (Record, error) {
	var r Record
	var summary, generated, scanned, counts, top string
	if err := scan(&r.AttemptID, &r.App, &r.Summary.Format, &r.Summary.PackageCount, &r.SBOMBytes, &r.HasProvenance, &summary, &generated,
		&r.ScanStatus, &r.Scanner, &r.ScanError, &scanned, &counts, &top, &r.GateAction, &r.GateReason); err != nil {
		return Record{}, err
	}
	if summary != "" {
		var sum SBOMSummary
		if err := json.Unmarshal([]byte(summary), &sum); err == nil {
			r.Summary = sum
		}
	}
	r.GeneratedAt, r.ScannedAt = parseTime(generated), parseTime(scanned)
	if counts != "" {
		sc := &ScanSummary{Scanner: r.Scanner}
		if err := json.Unmarshal([]byte(counts), &sc.Counts); err == nil {
			var d scanDetail
			if top != "" && json.Unmarshal([]byte(top), &d) == nil {
				sc.Fixable, sc.Top = d.Fixable, d.Top
			}
			r.Scan = sc
		}
	}
	return r, nil
}

// GetRecord returns one record or ErrNotFound.
func (s *SQLStore) GetRecord(ctx context.Context, attemptID string) (Record, error) {
	r, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM deploy_supply_chain WHERE attempt_id = ?`, attemptID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("store: get supply chain record %q: %w", attemptID, err)
	}
	return r, nil
}

func (s *SQLStore) queryRecords(ctx context.Context, query string, args ...any) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list supply chain records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Record
	for rows.Next() {
		r, err := scanRecord(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan supply chain record: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRecords returns app's records, newest first.
func (s *SQLStore) ListRecords(ctx context.Context, app string) ([]Record, error) {
	return s.queryRecords(ctx, `SELECT `+recordColumns+` FROM deploy_supply_chain WHERE app_name = ? ORDER BY generated_at DESC`, app)
}

// ListAllRecords returns every record.
func (s *SQLStore) ListAllRecords(ctx context.Context) ([]Record, error) {
	return s.queryRecords(ctx, `SELECT `+recordColumns+` FROM deploy_supply_chain`)
}

const lookupBatch = 200

// RecordsByAttempt maps attempt IDs to their records.
func (s *SQLStore) RecordsByAttempt(ctx context.Context, ids []string) (map[string]Record, error) {
	out := make(map[string]Record, len(ids))
	for start := 0; start < len(ids); start += lookupBatch {
		chunk := ids[start:min(start+lookupBatch, len(ids))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		recs, err := s.queryRecords(ctx, `SELECT `+recordColumns+` FROM deploy_supply_chain WHERE attempt_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			out[r.AttemptID] = r
		}
	}
	return out, nil
}

// DeleteRecords removes records by attempt ID.
func (s *SQLStore) DeleteRecords(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM deploy_supply_chain WHERE attempt_id = ?`, id); err != nil {
			return fmt.Errorf("store: delete supply chain record %q: %w", id, err)
		}
	}
	return nil
}

// DeleteApp removes every record and the settings of an app.
func (s *SQLStore) DeleteApp(ctx context.Context, app string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM deploy_supply_chain WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete supply chain records of %q: %w", app, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM app_supply_chain_settings WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete supply chain settings of %q: %w", app, err)
	}
	return nil
}

// AttemptExists reports whether a deploy attempt row exists.
func (s *SQLStore) AttemptExists(ctx context.Context, attemptID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM deploy_attempts WHERE id = ?`, attemptID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check deploy attempt %q: %w", attemptID, err)
	}
	return true, nil
}
