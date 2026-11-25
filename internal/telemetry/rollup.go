package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// RollupResult reports one Rollup run.
type RollupResult struct {
	Buckets int
	// More is true when a tier stopped at MaxBucketsPerRun with work left.
	More bool
}

// Rollup computes closed 1 minute buckets from raw samples and closed 1 hour
// buckets from the minute tier, at most MaxBucketsPerRun buckets per tier.
// Each chunk and its watermark commit in one transaction, and recomputing a
// bucket is idempotent, so an interrupted run resumes where it stopped.
func (db *DB) Rollup(ctx context.Context, now time.Time) (RollupResult, error) {
	cfg := db.tierConfig()
	var res RollupResult
	for _, tier := range []int{tierMinute, tierHour} {
		n, more, err := db.rollupTier(ctx, tier, now, cfg)
		if err != nil {
			return res, err
		}
		res.Buckets += n
		res.More = res.More || more
	}
	return res, nil
}

func (db *DB) rollupTier(ctx context.Context, tier int, now time.Time, cfg TierConfig) (buckets int, more bool, err error) {
	width := int64(tier)
	closedEnd := (now.Add(-cfg.Lag).Unix() / width) * width
	source, srcTier := "metric_samples", 0
	if tier == tierHour {
		source, srcTier = "metric_rollups", tierMinute
		srcDone, err := db.rollupWatermark(ctx, srcTier)
		if err != nil {
			return 0, false, err
		}
		if limit := srcDone / width * width; limit < closedEnd {
			closedEnd = limit
		}
	}

	start, err := db.rollupWatermark(ctx, tier)
	if err != nil {
		return 0, false, err
	}
	if start == 0 {
		start, err = db.oldestSourceTS(ctx, source, srcTier)
		if err != nil || start == 0 {
			return 0, false, err
		}
		start = start / width * width
	}
	if start >= closedEnd {
		return 0, false, nil
	}
	end := closedEnd
	if limit := start + int64(cfg.MaxBucketsPerRun)*width; limit < end {
		end, more = limit, true
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("telemetry: rollup %d: begin: %w", tier, err)
	}
	defer func() { _ = tx.Rollback() }()

	var query string
	var args []any
	if tier == tierMinute {
		in, inArgs := cumulativeIn()
		query = `
			INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
			SELECT ?, resource_id, metric, (ts / ?) * ?, SUM(value), MIN(value), MAX(value), COUNT(*)
			FROM metric_samples WHERE ts >= ? AND ts < ? AND metric NOT IN ` + in + `
			GROUP BY resource_id, metric, ts / ?
			ON CONFLICT (tier, resource_id, metric, ts) DO UPDATE SET
				sum = excluded.sum, min = excluded.min, max = excluded.max, count = excluded.count`
		args = append([]any{tier, width, width, start, end}, inArgs...)
		args = append(args, width)
		if err := db.rollupCumulative(ctx, tx, tier, start, end); err != nil {
			return 0, false, err
		}
	} else {
		query = `
			INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
			SELECT ?, resource_id, metric, (ts / ?) * ?, SUM(sum), MIN(min), MAX(max), SUM(count)
			FROM metric_rollups WHERE tier = ? AND ts >= ? AND ts < ?
			GROUP BY resource_id, metric, ts / ?
			ON CONFLICT (tier, resource_id, metric, ts) DO UPDATE SET
				sum = excluded.sum, min = excluded.min, max = excluded.max, count = excluded.count`
		args = []any{tier, width, width, tierMinute, start, end, width}
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return 0, false, fmt.Errorf("telemetry: rollup %d: aggregate: %w", tier, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rollup_state (tier, done_through) VALUES (?, ?)
		ON CONFLICT (tier) DO UPDATE SET done_through = excluded.done_through`, tier, end); err != nil {
		return 0, false, fmt.Errorf("telemetry: rollup %d: advance watermark: %w", tier, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("telemetry: rollup %d: commit: %w", tier, err)
	}
	return int((end - start) / width), more, nil
}

// minCumulativeLookback is the floor, in seconds, of the scan widening that keeps the delta across a bucket edge.
const minCumulativeLookback = 120

var cumulativeLookbackSource atomic.Pointer[func() time.Duration]

// SetCumulativeLookbackSource makes the lookback follow fn, so a saved sample or idle interval applies to the next run.
func SetCumulativeLookbackSource(fn func() time.Duration) { cumulativeLookbackSource.Store(&fn) }

// CumulativeLookback is twice the slowest gap between samples: the idle interval, or four times the live one.
func CumulativeLookback(idle, sample time.Duration) time.Duration {
	return 2 * max(idle, 4*sample)
}

// cumulativeLookbackSeconds never drops below the floor, so a missing source still bridges one bucket edge.
func cumulativeLookbackSeconds() int64 {
	secs := int64(minCumulativeLookback)
	if fn := cumulativeLookbackSource.Load(); fn != nil {
		secs = max(secs, int64((*fn)()/time.Second))
	}
	return secs
}

// rollupCumulative stores the reset aware increase per bucket: a negative step is a reset, so the new total counts whole.
func (db *DB) rollupCumulative(ctx context.Context, tx *sql.Tx, tier int, start, end int64) error {
	width := int64(tier)
	in, inArgs := cumulativeIn()
	args := append([]any{start - cumulativeLookbackSeconds(), end}, inArgs...)
	args = append([]any{tier, width, width}, args...)
	args = append(args, start, width)
	query := `
		INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
		SELECT ?, resource_id, metric, (ts / ?) * ?, SUM(inc), MIN(value), MAX(value), COUNT(*)
		FROM (
			SELECT resource_id, metric, ts, value,
				CASE WHEN prev IS NULL THEN 0 WHEN value >= prev THEN value - prev ELSE value END AS inc
			FROM (
				SELECT resource_id, metric, ts, value,
					LAG(value) OVER (PARTITION BY resource_id, metric ORDER BY ts) AS prev
				FROM metric_samples WHERE ts >= ? AND ts < ? AND metric IN {in}
			)
		) WHERE ts >= ?
		GROUP BY resource_id, metric, ts / ?
		ON CONFLICT (tier, resource_id, metric, ts) DO UPDATE SET
			sum = excluded.sum, min = excluded.min, max = excluded.max, count = excluded.count`
	_, err := tx.ExecContext(ctx, strings.Replace(query, "{in}", in, 1), args...)
	if err != nil {
		return fmt.Errorf("telemetry: rollup %d: cumulative increase: %w", tier, err)
	}
	return nil
}

func (db *DB) oldestSourceTS(ctx context.Context, table string, srcTier int) (int64, error) {
	var ts sql.NullInt64
	var err error
	if srcTier == 0 {
		err = db.QueryRowContext(ctx, `SELECT MIN(ts) FROM metric_samples`).Scan(&ts)
	} else {
		err = db.QueryRowContext(ctx, `SELECT MIN(ts) FROM metric_rollups WHERE tier = ?`, srcTier).Scan(&ts)
	}
	if err != nil {
		return 0, fmt.Errorf("telemetry: oldest ts in %s: %w", table, err)
	}
	return ts.Int64, nil
}

// PruneResult reports what one Prune run removed.
type PruneResult struct {
	Raw, Minute, Hour int64
	// OverCap is true when the size cap is still exceeded after pruning.
	OverCap bool
	// RawDroppedEarly is true when the size cap, not retention, removed raw rows.
	RawDroppedEarly bool
}

const sizePruneMaxPasses = 20

// Prune deletes samples and rollups past their retention and then, while the
// metric tables exceed MaxBytes, the oldest tenth of the finest tier that
// still has data already covered by the next tier, down to the headroom
// target. Raw rows are never dropped for size before they are rolled up.
func (db *DB) Prune(ctx context.Context, now time.Time) (PruneResult, error) {
	res, err := db.prune(ctx, now)
	// A passive checkpoint cannot pass an active reader, so it never blocks them.
	_, _ = db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	return res, err
}

func (db *DB) prune(ctx context.Context, now time.Time) (PruneResult, error) {
	cfg := db.tierConfig()
	var res PruneResult
	var err error
	deadline := time.Now().Add(currentRetentionTuning().SweepBudget)
	trafficCutoff := now.Add(-TrafficRetentionFromEnv(cfg.MinuteRetention)).Unix()
	if res.Raw, err = db.sweepBatched(ctx, rawSampleRetention, now.Add(-cfg.RawRetention).Unix(), deadline); err != nil {
		return res, err
	}
	if res.Minute, err = db.sweepBatched(ctx, minuteRollupRetention, now.Add(-cfg.MinuteRetention).Unix(), deadline); err != nil {
		return res, err
	}
	if res.Hour, err = db.sweepBatched(ctx, hourRollupRetention, now.Add(-cfg.HourRetention).Unix(), deadline); err != nil {
		return res, err
	}
	for _, target := range []retentionTarget{breakdownRetention, upstreamRetention, audienceRetention} {
		if _, err = db.sweepBatched(ctx, target, trafficCutoff, deadline); err != nil {
			return res, err
		}
	}
	if _, err = db.deleteBefore(ctx, `DELETE FROM request_samples WHERE ts < ?`, now.Add(-SampleRetentionFromEnv()).Unix()); err != nil {
		return res, err
	}
	if cfg.MaxBytes <= 0 {
		return res, nil
	}

	size, err := db.metricBytes(ctx)
	if err != nil {
		return res, err
	}
	if size > cfg.MaxBytes {
		target := cfg.MaxBytes / 100 * int64(currentCapTargetPercent())
		for pass := 0; pass < sizePruneMaxPasses && size > target; pass++ {
			n, err := db.pruneOldestTenth(ctx)
			if err != nil {
				return res, err
			}
			res.Raw += n[0]
			res.Minute += n[1]
			res.Hour += n[2]
			res.RawDroppedEarly = res.RawDroppedEarly || n[0] > 0
			if n[0]+n[1]+n[2] == 0 {
				break
			}
			if size, err = db.metricBytes(ctx); err != nil {
				return res, err
			}
		}
	}
	res.OverCap = size > cfg.MaxBytes
	return res, nil
}

func (db *DB) deleteBefore(ctx context.Context, query string, cutoff int64) (int64, error) {
	r, err := db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("telemetry: prune: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: prune rows affected: %w", err)
	}
	return n, nil
}

func (db *DB) pruneOldestTenth(ctx context.Context) ([3]int64, error) {
	var out [3]int64
	rawDone, err := db.rollupWatermark(ctx, tierMinute)
	if err != nil {
		return out, err
	}
	// Keep the lookback of raw below the watermark so the next bucket still finds its previous total.
	rawDone -= cumulativeLookbackSeconds()
	minuteDone, err := db.rollupWatermark(ctx, tierHour)
	if err != nil {
		return out, err
	}
	var newestHour sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(ts) FROM metric_rollups WHERE tier = 3600`).Scan(&newestHour); err != nil {
		return out, fmt.Errorf("telemetry: prune newest hour: %w", err)
	}
	steps := []struct {
		minQuery string
		delQuery string
		limit    int64
	}{
		{`SELECT MIN(ts) FROM metric_samples`, `DELETE FROM metric_samples WHERE ts < ?`, rawDone},
		{`SELECT MIN(ts) FROM metric_rollups WHERE tier = 60`, `DELETE FROM metric_rollups WHERE tier = 60 AND ts < ?`, minuteDone},
		{`SELECT MIN(ts) FROM metric_rollups WHERE tier = 3600`, `DELETE FROM metric_rollups WHERE tier = 3600 AND ts < ?`, newestHour.Int64},
	}
	for i, st := range steps {
		var oldest sql.NullInt64
		if err := db.QueryRowContext(ctx, st.minQuery).Scan(&oldest); err != nil {
			return out, fmt.Errorf("telemetry: prune oldest: %w", err)
		}
		if !oldest.Valid || oldest.Int64 >= st.limit {
			continue
		}
		cutoff := min(oldest.Int64+(st.limit-oldest.Int64)/10+1, st.limit)
		n, err := db.deleteBefore(ctx, st.delQuery, cutoff)
		if err != nil {
			return out, err
		}
		out[i] = n
		if n > 0 {
			return out, nil
		}
	}
	return out, nil
}

// Maintainer runs rollups and pruning on timers.
type Maintainer struct {
	db          *DB
	logger      *slog.Logger
	rollupEvery time.Duration
	pruneEvery  time.Duration
}

// NewMaintainer builds a Maintainer; zero intervals default to 1 minute for
// rollups and 1 hour for pruning.
func NewMaintainer(db *DB, rollupEvery, pruneEvery time.Duration, logger *slog.Logger) *Maintainer {
	if rollupEvery <= 0 {
		rollupEvery = time.Minute
	}
	if pruneEvery <= 0 {
		pruneEvery = time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Maintainer{db: db, logger: logger, rollupEvery: rollupEvery, pruneEvery: pruneEvery}
}

const maxCatchUpRuns = 20

// RollupOnce runs Rollup, repeating (bounded) while a tier has backlog.
func (m *Maintainer) RollupOnce(ctx context.Context) error {
	for i := 0; i < maxCatchUpRuns; i++ {
		res, err := m.db.Rollup(ctx, m.db.now())
		if err != nil {
			return err
		}
		if !res.More {
			return nil
		}
	}
	return nil
}

// PruneOnce runs Prune and logs what it removed.
func (m *Maintainer) PruneOnce(ctx context.Context) error {
	res, err := m.db.Prune(ctx, m.db.now())
	if err != nil {
		return err
	}
	if res.Raw+res.Minute+res.Hour > 0 {
		m.logger.InfoContext(ctx, "metrics retention sweep",
			slog.Int64("raw", res.Raw), slog.Int64("minute", res.Minute), slog.Int64("hour", res.Hour))
	}
	if res.OverCap {
		m.logger.WarnContext(ctx, "metrics tables still over size cap after pruning")
	}
	if res.RawDroppedEarly {
		m.logger.WarnContext(ctx, "metrics size cap dropped raw data before retention, raise the cap or shorten retention")
	}
	return nil
}

// Run blocks until ctx is cancelled, rolling up and pruning on their timers.
func (m *Maintainer) Run(ctx context.Context) {
	rollup := time.NewTicker(m.rollupEvery)
	prune := time.NewTicker(m.pruneEvery)
	defer rollup.Stop()
	defer prune.Stop()
	rollupLoop := footprint.Track("telemetry rollup", m.rollupEvery)
	pruneLoop := footprint.Track("telemetry prune", m.pruneEvery)
	defer rollupLoop.Active()()
	defer pruneLoop.Active()()
	for {
		select {
		case <-ctx.Done():
			return
		case <-rollup.C:
			start := time.Now()
			err := m.RollupOnce(ctx)
			rollupLoop.Observe(start, err)
			if err != nil && !errors.Is(err, context.Canceled) {
				m.logger.ErrorContext(ctx, "metrics rollup failed", slog.String("error", err.Error()))
			}
		case <-prune.C:
			start := time.Now()
			err := m.PruneOnce(ctx)
			pruneLoop.Observe(start, err)
			if err != nil && !errors.Is(err, context.Canceled) {
				m.logger.ErrorContext(ctx, "metrics prune failed", slog.String("error", err.Error()))
			}
		}
	}
}
