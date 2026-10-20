package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CanaryServiceSuffix marks the hidden clone service that runs a canary.
const CanaryServiceSuffix = "--canary"

// CanaryServiceName returns the hidden service name that runs serviceName's canary.
func CanaryServiceName(serviceName string) string { return serviceName + CanaryServiceSuffix }

// IsCanaryService reports whether name is a hidden canary clone.
func IsCanaryService(name string) bool { return strings.HasSuffix(name, CanaryServiceSuffix) }

// CanaryRelease is an app's in-flight canary (migrations/0378_canary_releases.sql).
// A release with Stages is staged: the canary reconciler walks Weight through
// Stages and rolls back on a breach. Without Stages the operator drives Weight.
type CanaryRelease struct {
	ServiceName   string
	CanaryService string
	Image         string
	Weight        int
	CreatedAt     time.Time
	Stages        []int
	StageIndex    int
	IntervalSecs  int
	MaxErrorRate  float64
	MaxP95Ratio   float64
	StatusReason  string
}

// Staged reports whether the canary reconciler owns this release's weight.
func (c CanaryRelease) Staged() bool { return len(c.Stages) > 0 }

// CanaryPolicy is an app's saved canary defaults, written from app.yaml.
type CanaryPolicy struct {
	ServiceName  string
	Stages       []int
	IntervalSecs int
	MaxErrorRate float64
	MaxP95Ratio  float64
}

// Canary outcomes recorded in CanaryOutcome.Outcome.
const (
	CanaryOutcomePromoted   = "promoted"
	CanaryOutcomeAborted    = "aborted"
	CanaryOutcomeRolledBack = "rolled_back"
)

// CanaryOutcome is how an app's last canary ended.
type CanaryOutcome struct {
	ServiceName string
	Outcome     string
	Reason      string
	Image       string
	At          time.Time
}

func joinStages(stages []int) string {
	parts := make([]string, len(stages))
	for i, s := range stages {
		parts[i] = strconv.Itoa(s)
	}
	return strings.Join(parts, ",")
}

func splitStages(v string) []int {
	if v == "" {
		return nil
	}
	var out []int
	for _, p := range strings.Split(v, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// SetCanaryRelease creates or replaces serviceName's canary row.
func (db *DB) SetCanaryRelease(ctx context.Context, c CanaryRelease) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canary_releases (service_name, canary_service, image, weight, created_at, stages, stage_index, interval_secs, max_error_rate, max_p95_ratio, status_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET canary_service = excluded.canary_service, image = excluded.image, weight = excluded.weight,
			stages = excluded.stages, stage_index = excluded.stage_index, interval_secs = excluded.interval_secs,
			max_error_rate = excluded.max_error_rate, max_p95_ratio = excluded.max_p95_ratio, status_reason = excluded.status_reason
	`, c.ServiceName, c.CanaryService, c.Image, c.Weight, c.CreatedAt.UTC().Format(time.RFC3339),
		joinStages(c.Stages), c.StageIndex, c.IntervalSecs, c.MaxErrorRate, c.MaxP95Ratio, c.StatusReason)
	if err != nil {
		return fmt.Errorf("store: set canary release for %q: %w", c.ServiceName, err)
	}
	return nil
}

const canaryColumns = `service_name, canary_service, image, weight, created_at, stages, stage_index, interval_secs, max_error_rate, max_p95_ratio, status_reason`

type canaryScanner interface{ Scan(dest ...any) error }

func scanCanary(row canaryScanner) (CanaryRelease, error) {
	var c CanaryRelease
	var created, stages string
	if err := row.Scan(&c.ServiceName, &c.CanaryService, &c.Image, &c.Weight, &created, &stages, &c.StageIndex, &c.IntervalSecs, &c.MaxErrorRate, &c.MaxP95Ratio, &c.StatusReason); err != nil {
		return CanaryRelease{}, err
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return CanaryRelease{}, err
	}
	c.CreatedAt, c.Stages = t, splitStages(stages)
	return c, nil
}

// GetCanaryRelease returns serviceName's canary and whether one exists.
func (db *DB) GetCanaryRelease(ctx context.Context, serviceName string) (CanaryRelease, bool, error) {
	c, err := scanCanary(db.QueryRowContext(ctx, `SELECT `+canaryColumns+` FROM canary_releases WHERE service_name = ?`, serviceName))
	if errors.Is(err, sql.ErrNoRows) {
		return CanaryRelease{}, false, nil
	}
	if err != nil {
		return CanaryRelease{}, false, fmt.Errorf("store: get canary release for %q: %w", serviceName, err)
	}
	return c, true, nil
}

// DeleteCanaryRelease removes serviceName's canary row; a missing row is not an error.
func (db *DB) DeleteCanaryRelease(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM canary_releases WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete canary release for %q: %w", serviceName, err)
	}
	return nil
}

// ListCanaryReleases returns every in-flight canary.
func (db *DB) ListCanaryReleases(ctx context.Context) ([]CanaryRelease, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+canaryColumns+` FROM canary_releases ORDER BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list canary releases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CanaryRelease
	for rows.Next() {
		c, err := scanCanary(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list canary releases: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCanaryPolicy saves serviceName's canary defaults.
func (db *DB) SetCanaryPolicy(ctx context.Context, p CanaryPolicy) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO canary_policies (service_name, stages, interval_secs, max_error_rate, max_p95_ratio) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET stages = excluded.stages, interval_secs = excluded.interval_secs,
			max_error_rate = excluded.max_error_rate, max_p95_ratio = excluded.max_p95_ratio
	`, p.ServiceName, joinStages(p.Stages), p.IntervalSecs, p.MaxErrorRate, p.MaxP95Ratio)
	if err != nil {
		return fmt.Errorf("store: set canary policy for %q: %w", p.ServiceName, err)
	}
	return nil
}

// GetCanaryPolicy returns serviceName's saved canary defaults and whether any exist.
func (db *DB) GetCanaryPolicy(ctx context.Context, serviceName string) (CanaryPolicy, bool, error) {
	var p CanaryPolicy
	var stages string
	err := db.QueryRowContext(ctx, `
		SELECT service_name, stages, interval_secs, max_error_rate, max_p95_ratio FROM canary_policies WHERE service_name = ?
	`, serviceName).Scan(&p.ServiceName, &stages, &p.IntervalSecs, &p.MaxErrorRate, &p.MaxP95Ratio)
	if errors.Is(err, sql.ErrNoRows) {
		return CanaryPolicy{}, false, nil
	}
	if err != nil {
		return CanaryPolicy{}, false, fmt.Errorf("store: get canary policy for %q: %w", serviceName, err)
	}
	p.Stages = splitStages(stages)
	return p, true, nil
}

// DeleteCanaryPolicy removes serviceName's canary defaults; a missing row is not an error.
func (db *DB) DeleteCanaryPolicy(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM canary_policies WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete canary policy for %q: %w", serviceName, err)
	}
	return nil
}

// SetCanaryOutcome records how serviceName's last canary ended.
func (db *DB) SetCanaryOutcome(ctx context.Context, o CanaryOutcome) error {
	if o.At.IsZero() {
		o.At = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO canary_outcomes (service_name, outcome, reason, image, at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET outcome = excluded.outcome, reason = excluded.reason, image = excluded.image, at = excluded.at
	`, o.ServiceName, o.Outcome, o.Reason, o.Image, o.At.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set canary outcome for %q: %w", o.ServiceName, err)
	}
	return nil
}

// GetCanaryOutcome returns serviceName's last canary outcome and whether one exists.
func (db *DB) GetCanaryOutcome(ctx context.Context, serviceName string) (CanaryOutcome, bool, error) {
	var o CanaryOutcome
	var at string
	err := db.QueryRowContext(ctx, `
		SELECT service_name, outcome, reason, image, at FROM canary_outcomes WHERE service_name = ?
	`, serviceName).Scan(&o.ServiceName, &o.Outcome, &o.Reason, &o.Image, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return CanaryOutcome{}, false, nil
	}
	if err != nil {
		return CanaryOutcome{}, false, fmt.Errorf("store: get canary outcome for %q: %w", serviceName, err)
	}
	if o.At, err = time.Parse(time.RFC3339, at); err != nil {
		return CanaryOutcome{}, false, fmt.Errorf("store: get canary outcome for %q: %w", serviceName, err)
	}
	return o, true, nil
}
