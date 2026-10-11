package telemetry

import (
	"context"
	"fmt"
	"time"
)

const (
	detectMinSpan = 10 * time.Minute
	detectMaxSpan = 24 * time.Hour
)

// NewSignatureDetector counts error signatures that showed up after a deploy
// and did not exist in the equally long window before it.
type NewSignatureDetector struct {
	Src BoundedLogQuerier
	// MaxRows bounds each of the two reads.
	MaxRows int
}

// NewErrorTypes compares [deployedAt, until] (clamped to between ten minutes
// and a day) with the same span before deployedAt.
func (d NewSignatureDetector) NewErrorTypes(ctx context.Context, app string, deployedAt, until time.Time) (int, error) {
	span := until.Sub(deployedAt)
	if span > detectMaxSpan {
		span = detectMaxSpan
	}
	if span < detectMinSpan {
		span = detectMinSpan
	}
	rows := d.MaxRows
	if rows <= 0 {
		rows = 20000
	}
	resource := "service:" + app
	opts := SignatureOptions{MinLevel: LevelError, Buckets: 1}

	after, _, err := QueryLogsBounded(ctx, d.Src, resource, deployedAt, deployedAt.Add(span), "", rows)
	if err != nil && len(after) == 0 {
		return 0, fmt.Errorf("telemetry: new error types for %s: read after deploy: %w", app, err)
	}
	cur := AggregateSignatures(after, deployedAt, deployedAt.Add(span), opts)
	if len(cur) == 0 {
		return 0, nil
	}
	before, _, err := QueryLogsBounded(ctx, d.Src, resource, deployedAt.Add(-span), deployedAt, "", rows)
	if err != nil && len(before) == 0 {
		return 0, fmt.Errorf("telemetry: new error types for %s: read before deploy: %w", app, err)
	}
	return MarkNewSignatures(cur, AggregateSignatures(before, deployedAt.Add(-span), deployedAt, opts)), nil
}
