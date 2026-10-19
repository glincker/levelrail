package objectstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BucketStore is the bucket row surface the sweeper needs.
type BucketStore interface {
	ListStorageBuckets(ctx context.Context) ([]store.StorageBucket, error)
	MarkStorageBucketSwept(ctx context.Context, id string, at time.Time) error
}

// BucketSweeper expires objects in buckets that have a lifecycle rule.
type BucketSweeper struct {
	Store    BucketStore
	Resolver *Resolver
	Logger   *slog.Logger
	Tick     time.Duration
	Now      func() time.Time
}

// Run sweeps every Tick (default hourly) until ctx ends. Buckets with no rule cost one list query.
func (s *BucketSweeper) Run(ctx context.Context) {
	tick := s.Tick
	if tick <= 0 {
		tick = time.Hour
	}
	loop := footprint.Track("storage bucket sweep", tick)
	defer loop.Active()()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			start := time.Now()
			s.SweepAll(ctx)
			loop.Observe(start, nil)
		}
	}
}

// SweepAll runs the lifecycle rule of every bucket that has one.
func (s *BucketSweeper) SweepAll(ctx context.Context) {
	buckets, err := s.Store.ListStorageBuckets(ctx)
	if err != nil {
		s.Logger.Error("storage buckets: list for sweep failed", slog.String("error", err.Error()))
		return
	}
	for _, b := range buckets {
		if ctx.Err() != nil {
			return
		}
		if b.LifecycleDays <= 0 {
			continue
		}
		if _, err := s.Sweep(ctx, b); err != nil {
			s.Logger.Warn("storage buckets: sweep failed", slog.String("bucket_id", b.ID), slog.String("error", err.Error()))
		}
	}
}

// Sweep deletes objects in b older than its lifecycle days.
func (s *BucketSweeper) Sweep(ctx context.Context, b store.StorageBucket) (SweepResult, error) {
	if b.LifecycleDays <= 0 {
		return SweepResult{}, fmt.Errorf("objectstore: bucket %q has no lifecycle rule", b.ID)
	}
	client, _, err := s.Resolver.Client(ctx, b.TargetID)
	if err != nil {
		return SweepResult{}, err
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	res, err := client.SweepExpired(ctx, b.Prefix, now.AddDate(0, 0, -b.LifecycleDays))
	if err != nil {
		return res, err
	}
	if err := s.Store.MarkStorageBucketSwept(ctx, b.ID, now); err != nil {
		return res, fmt.Errorf("objectstore: %w", err)
	}
	return res, nil
}
