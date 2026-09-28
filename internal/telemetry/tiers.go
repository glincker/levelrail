package telemetry

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	tierMinute = 60
	tierHour   = 3600
)

// TierConfig controls rollup tiers, retention and query tier selection.
type TierConfig struct {
	RawRetention    time.Duration
	MinuteRetention time.Duration
	HourRetention   time.Duration
	// RawMaxRange and MinuteMaxRange are the widest query ranges served
	// from the raw and 1 minute tiers; wider ranges use the next tier.
	RawMaxRange    time.Duration
	MinuteMaxRange time.Duration
	// MaxBytes caps the live size of the database file; 0 disables it.
	MaxBytes int64
	// Lag delays closing a bucket so late samples still land in it.
	Lag time.Duration
	// MaxBucketsPerRun bounds the buckets one rollup run computes per tier.
	MaxBucketsPerRun int
}

// DefaultTierConfig returns the documented defaults.
func DefaultTierConfig() TierConfig {
	return TierConfig{
		RawRetention:     15 * 24 * time.Hour,
		MinuteRetention:  30 * 24 * time.Hour,
		HourRetention:    365 * 24 * time.Hour,
		RawMaxRange:      6 * time.Hour,
		MinuteMaxRange:   7 * 24 * time.Hour,
		MaxBytes:         1 << 30,
		Lag:              30 * time.Second,
		MaxBucketsPerRun: 1440,
	}
}

// TierConfigFromEnv reads the APP_METRICS_* overrides through getenv and
// falls back to DefaultTierConfig for unset or invalid values.
func TierConfigFromEnv(getenv func(string) string) TierConfig {
	cfg := DefaultTierConfig()
	dur := func(key string, dst *time.Duration) {
		if d, err := time.ParseDuration(getenv(key)); err == nil && d > 0 {
			*dst = d
		}
	}
	dur("APP_METRICS_RETENTION", &cfg.RawRetention)
	dur("APP_METRICS_RETENTION_1M", &cfg.MinuteRetention)
	dur("APP_METRICS_RETENTION_1H", &cfg.HourRetention)
	dur("APP_METRICS_RAW_QUERY_MAX_RANGE", &cfg.RawMaxRange)
	dur("APP_METRICS_MINUTE_QUERY_MAX_RANGE", &cfg.MinuteMaxRange)
	dur("APP_METRICS_ROLLUP_LAG", &cfg.Lag)
	if n, err := strconv.ParseInt(getenv("APP_METRICS_MAX_DB_BYTES"), 10, 64); err == nil && n >= 0 {
		cfg.MaxBytes = n
	}
	if n, err := strconv.Atoi(getenv("APP_METRICS_ROLLUP_MAX_BUCKETS")); err == nil && n > 0 {
		cfg.MaxBucketsPerRun = n
	}
	return cfg
}

// Configure sets the tier configuration. Call it before the DB is shared
// with other goroutines.
func (db *DB) Configure(cfg TierConfig) { db.tiers = &cfg }

// SetClock replaces the time source used for tier selection and
// maintenance; tests use it to drive a fake clock.
func (db *DB) SetClock(now func() time.Time) { db.clock = now }

func (db *DB) tierConfig() TierConfig {
	if db.tiers == nil {
		return DefaultTierConfig()
	}
	return *db.tiers
}

func (db *DB) now() time.Time {
	if db.clock != nil {
		return db.clock().UTC()
	}
	return time.Now().UTC()
}

// pickTier returns the coarsest tier the range calls for, moving to a
// coarser one when the finer tier no longer retains the range start.
func (db *DB) pickTier(from, to time.Time) int {
	cfg := db.tierConfig()
	now := db.now()
	span := to.Sub(from)
	tier := 0
	switch {
	case span > cfg.MinuteMaxRange:
		tier = tierHour
	case span > cfg.RawMaxRange:
		tier = tierMinute
	}
	if tier == 0 && from.Before(now.Add(-cfg.RawRetention)) {
		tier = tierMinute
	}
	if tier == tierMinute && from.Before(now.Add(-cfg.MinuteRetention)) {
		tier = tierHour
	}
	return tier
}

// Query returns samples for resourceID/metric in [from, to], oldest first,
// reading the tier the range calls for. Rollup tiers return one point per
// bucket (sum for counters, average for gauges) and are topped up with the
// finer tier for the not yet rolled up tail.
func (db *DB) Query(ctx context.Context, resourceID, metric string, from, to time.Time) ([]Sample, error) {
	return db.queryTier(ctx, db.pickTier(from, to), resourceID, metric, from, to)
}

func (db *DB) queryTier(ctx context.Context, tier int, resourceID, metric string, from, to time.Time) ([]Sample, error) {
	if tier == 0 {
		return db.queryRaw(ctx, resourceID, metric, from, to)
	}
	done, err := db.rollupWatermark(ctx, tier)
	if err != nil {
		return nil, err
	}
	var out []Sample
	head := to
	if done > 0 && time.Unix(done, 0).Before(to) {
		head = time.Unix(done, 0).Add(-time.Second)
	}
	if done > 0 && !head.Before(from) {
		out, err = db.queryRollup(ctx, tier, resourceID, metric, from, head)
		if err != nil {
			return nil, err
		}
	}
	tailFrom := from
	if done > 0 && time.Unix(done, 0).After(from) {
		tailFrom = time.Unix(done, 0)
	}
	if tailFrom.After(to) {
		return out, nil
	}
	finer := 0
	if tier == tierHour {
		finer = tierMinute
	}
	tail, err := db.queryTier(ctx, finer, resourceID, metric, tailFrom, to)
	if err != nil {
		return nil, err
	}
	return append(out, tail...), nil
}

func (db *DB) queryRollup(ctx context.Context, tier int, resourceID, metric string, from, to time.Time) ([]Sample, error) {
	width := int64(tier)
	lo := from.Unix() / width * width
	rows, err := db.QueryContext(ctx, `
		SELECT ts, sum, count FROM metric_rollups
		WHERE tier = ? AND resource_id = ? AND metric = ? AND ts BETWEEN ? AND ?
		ORDER BY ts ASC
	`, tier, resourceID, metric, lo, to.Unix())
	if err != nil {
		return nil, fmt.Errorf("telemetry: query rollup %s/%s: %w", resourceID, metric, err)
	}
	defer func() { _ = rows.Close() }()

	sumKind := IsSumMetric(metric)
	var out []Sample
	for rows.Next() {
		var ts, count int64
		var sum float64
		if err := rows.Scan(&ts, &sum, &count); err != nil {
			return nil, fmt.Errorf("telemetry: scan rollup row: %w", err)
		}
		v := sum
		if !sumKind && count > 0 {
			v = sum / float64(count)
		}
		out = append(out, Sample{ResourceID: resourceID, Metric: metric, Timestamp: time.Unix(ts, 0).UTC(), Value: v})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate rollup rows: %w", err)
	}
	return out, nil
}

func (db *DB) rollupWatermark(ctx context.Context, tier int) (int64, error) {
	var done int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE((SELECT done_through FROM rollup_state WHERE tier = ?), 0)`, tier).Scan(&done)
	if err != nil {
		return 0, fmt.Errorf("telemetry: read rollup watermark %d: %w", tier, err)
	}
	return done, nil
}
