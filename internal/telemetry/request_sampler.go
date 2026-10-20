package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// RequestDrainer hands over and resets request activity accumulated since the
// last call, keyed by app name. *ingress.RequestStats satisfies it.
type RequestDrainer interface {
	DrainByApp() map[string]RequestWindow
}

// RequestSampler writes drained request activity as samples on a fixed tick.
type RequestSampler struct {
	src      RequestDrainer
	db       *DB
	interval time.Duration
	logger   *slog.Logger
}

// NewRequestSampler builds a sampler; a nil logger falls back to slog.Default.
func NewRequestSampler(src RequestDrainer, db *DB, interval time.Duration, logger *slog.Logger) *RequestSampler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RequestSampler{src: src, db: db, interval: interval, logger: logger}
}

// SampleOnce drains the source and stores the result at time at.
func (s *RequestSampler) SampleOnce(ctx context.Context, at time.Time) error {
	return s.db.RecordRequests(ctx, s.src.DrainByApp(), at)
}

// Run samples every interval until ctx is cancelled.
func (s *RequestSampler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.SampleOnce(ctx, now.UTC().Truncate(time.Second)); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.ErrorContext(ctx, "request metrics sample failed", slog.String("error", err.Error()))
			}
		}
	}
}
