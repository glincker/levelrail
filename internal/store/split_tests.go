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

	"github.com/GLINCKER/levelrail/internal/splittest"
)

// ErrSplitTestRunning is returned when an app already has a running test.
var ErrSplitTestRunning = errors.New("store: a split test is already running for this app")

// SplitTest is one split test of an app (migrations/2690_split_tests.sql).
type SplitTest struct {
	ID            string
	AppName       string
	Status        string
	Variants      []splittest.Variant
	CookieName    string
	CookieTTLSecs int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	EndedAt       time.Time
	EndReason     string
}

// SplitStat is one variant's request count in one hour.
type SplitStat struct {
	VariantID string
	Bucket    time.Time
	Requests  uint64
}

// NewSplitTestID returns a random test ID.
func NewSplitTestID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: split test id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

const splitTestCols = `id, app_name, status, variants, cookie_name, cookie_ttl_secs, created_at, updated_at, ended_at, end_reason`

func scanSplitTest(scan func(dest ...any) error) (SplitTest, error) {
	var t SplitTest
	var raw, created, updated, ended string
	if err := scan(&t.ID, &t.AppName, &t.Status, &raw, &t.CookieName, &t.CookieTTLSecs, &created, &updated, &ended, &t.EndReason); err != nil {
		return SplitTest{}, err
	}
	if err := json.Unmarshal([]byte(raw), &t.Variants); err != nil {
		return SplitTest{}, fmt.Errorf("decode variants of split test %q: %w", t.ID, err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if ended != "" {
		t.EndedAt, _ = time.Parse(time.RFC3339Nano, ended)
	}
	return t, nil
}

// CreateSplitTest inserts a running test. A second running test for the same
// app fails with ErrSplitTestRunning.
func (db *DB) CreateSplitTest(ctx context.Context, t SplitTest) error {
	raw, err := json.Marshal(t.Variants)
	if err != nil {
		return fmt.Errorf("store: marshal split variants: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `INSERT INTO split_tests (`+splitTestCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', '')`,
		t.ID, t.AppName, t.Status, string(raw), t.CookieName, t.CookieTTLSecs, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrSplitTestRunning
		}
		return fmt.Errorf("store: create split test: %w", err)
	}
	return nil
}

// GetRunningSplitTest returns the app's running test.
func (db *DB) GetRunningSplitTest(ctx context.Context, app string) (SplitTest, bool, error) {
	t, err := scanSplitTest(db.QueryRowContext(ctx, `SELECT `+splitTestCols+` FROM split_tests WHERE app_name = ? AND status = ?`, app, splittest.StatusRunning).Scan)
	return splitTestResult(t, err)
}

// GetLatestSplitTest returns the app's newest test, running or ended.
func (db *DB) GetLatestSplitTest(ctx context.Context, app string) (SplitTest, bool, error) {
	t, err := scanSplitTest(db.QueryRowContext(ctx, `SELECT `+splitTestCols+` FROM split_tests WHERE app_name = ? ORDER BY created_at DESC LIMIT 1`, app).Scan)
	return splitTestResult(t, err)
}

func splitTestResult(t SplitTest, err error) (SplitTest, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return SplitTest{}, false, nil
	}
	if err != nil {
		return SplitTest{}, false, fmt.Errorf("store: load split test: %w", err)
	}
	return t, true, nil
}

// ListRunningSplitTests returns every running test, for rebuilding ingress at boot.
func (db *DB) ListRunningSplitTests(ctx context.Context) ([]SplitTest, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+splitTestCols+` FROM split_tests WHERE status = ?`, splittest.StatusRunning)
	if err != nil {
		return nil, fmt.Errorf("store: list running split tests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SplitTest
	for rows.Next() {
		t, err := scanSplitTest(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan split test: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateSplitTestVariants replaces the variants (weights) of a running test.
func (db *DB) UpdateSplitTestVariants(ctx context.Context, id string, vs []splittest.Variant) error {
	raw, err := json.Marshal(vs)
	if err != nil {
		return fmt.Errorf("store: marshal split variants: %w", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE split_tests SET variants = ?, updated_at = ? WHERE id = ? AND status = ?`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano), id, splittest.StatusRunning); err != nil {
		return fmt.Errorf("store: update split test %q: %w", id, err)
	}
	return nil
}

// EndSplitTest marks a test stopped and records why.
func (db *DB) EndSplitTest(ctx context.Context, id, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `UPDATE split_tests SET status = ?, ended_at = ?, updated_at = ?, end_reason = ? WHERE id = ?`,
		splittest.StatusStopped, now, now, reason, id); err != nil {
		return fmt.Errorf("store: end split test %q: %w", id, err)
	}
	return nil
}

// AddSplitStats adds request counts into the hour bucket containing at.
func (db *DB) AddSplitStats(ctx context.Context, testID string, counts map[string]uint64, at time.Time) error {
	bucket := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	for variant, n := range counts {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO split_test_stats (test_id, variant_id, bucket, requests) VALUES (?, ?, ?, ?)
			ON CONFLICT (test_id, variant_id, bucket) DO UPDATE SET requests = requests + excluded.requests
		`, testID, variant, bucket, int64(n)); err != nil { //nolint:gosec // request counts stay far below int64 max
			return fmt.Errorf("store: add split stats %q: %w", testID, err)
		}
	}
	return nil
}

// ListSplitStats returns a test's hourly counts, oldest first.
func (db *DB) ListSplitStats(ctx context.Context, testID string) ([]SplitStat, error) {
	rows, err := db.QueryContext(ctx, `SELECT variant_id, bucket, requests FROM split_test_stats WHERE test_id = ? ORDER BY bucket, variant_id`, testID)
	if err != nil {
		return nil, fmt.Errorf("store: list split stats: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SplitStat
	for rows.Next() {
		var s SplitStat
		var bucket string
		var n int64
		if err := rows.Scan(&s.VariantID, &bucket, &n); err != nil {
			return nil, fmt.Errorf("store: scan split stat: %w", err)
		}
		s.Bucket, _ = time.Parse(time.RFC3339, bucket)
		s.Requests = uint64(n) //nolint:gosec // written from uint64 counts, never negative
		out = append(out, s)
	}
	return out, rows.Err()
}
