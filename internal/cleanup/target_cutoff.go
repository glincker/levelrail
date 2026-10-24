package cleanup

import (
	"context"
	"fmt"
	"time"
)

const hoursPerDayDuration = hoursPerDay * time.Hour

// cutoffTarget is a table cleaned by deleting rows older than a cutoff with an existing store function.
type cutoffTarget struct {
	meta    Meta
	now     func() time.Time
	measure func(ctx context.Context) (Usage, error)
	count   func(ctx context.Context, cutoff time.Time, days int) (int64, error)
	del     func(ctx context.Context, cutoff time.Time, days int) (int64, error)
}

func (t *cutoffTarget) Meta() Meta { return t.meta }

func (t *cutoffTarget) Measure(ctx context.Context) (Usage, error) { return t.measure(ctx) }

func (t *cutoffTarget) cutoff(days int) time.Time {
	return t.now().Add(-time.Duration(days) * hoursPerDayDuration)
}

// Preview counts what Run would delete for the same window; bytes is the table's average row size times that count.
func (t *cutoffTarget) Preview(ctx context.Context, days int) (Impact, error) {
	n, err := t.count(ctx, t.cutoff(days), days)
	if err != nil {
		return Impact{}, err
	}
	u, merr := t.measure(ctx)
	imp := Impact{Rows: n}
	if merr == nil && u.Rows > 0 {
		imp.Bytes = n * (u.Bytes / u.Rows)
	}
	return imp, nil
}

func (t *cutoffTarget) Run(ctx context.Context, days int, opts RunOptions) Result {
	if opts.DryRun {
		imp, err := t.Preview(ctx, days)
		return Result{Rows: imp.Rows, Bytes: imp.Bytes, Err: err}
	}
	u, err := t.measure(ctx)
	if err != nil {
		return Result{Err: fmt.Errorf("cleanup: measure %s: %w", t.meta.ID, err)}
	}
	var avg int64
	if u.Rows > 0 {
		avg = u.Bytes / u.Rows
	}
	oldest := time.Time{}
	if u.Oldest != nil {
		oldest = *u.Oldest
	}
	rows, more, err := SweepByCutoff(ctx, oldest, t.cutoff(days), opts.Slice, opts.MaxRows, func(ctx context.Context, cutoff time.Time) (int64, error) {
		return t.del(ctx, cutoff, days)
	})
	res := Result{Rows: rows, Bytes: rows * avg, More: more}
	if err != nil {
		res.Err = fmt.Errorf("cleanup: %s: %w", t.meta.ID, err)
	}
	return res
}

func usageOf(rows, bytes int64, oldest time.Time) Usage {
	u := Usage{Rows: rows, Bytes: bytes}
	if !oldest.IsZero() {
		o := oldest
		u.Oldest = &o
	}
	return u
}
