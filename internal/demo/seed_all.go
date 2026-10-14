package demo

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Stores groups the three SQLite files a demo estate spans.
type Stores struct {
	DB        *store.DB
	Telemetry *telemetry.DB
	Alerting  *alerting.DB
}

// SeedAll seeds the relational store, then telemetry and alerting. A store
// that already carries a matching marker is left untouched, so a restart
// never duplicates log lines.
func SeedAll(ctx context.Context, st Stores, opts Options) (Manifest, error) {
	_, err := st.DB.GetDemoMarker(ctx)
	switch {
	case err == nil:
		return Seed(ctx, st.DB, opts)
	case !errors.Is(err, store.ErrDemoMarkerNotFound):
		return Manifest{}, fmt.Errorf("demo: read marker: %w", err)
	}
	m, err := Seed(ctx, st.DB, opts)
	if err != nil {
		return Manifest{}, err
	}
	if opts.Anchor.IsZero() {
		return m, nil
	}
	if err := SeedTelemetry(ctx, st.DB, st.Telemetry, opts); err != nil {
		return Manifest{}, err
	}
	if err := SeedAlerting(ctx, st.Alerting, opts); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
