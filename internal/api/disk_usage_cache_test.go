package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type countingDiskUsager struct {
	calls int
	err   error
}

func (c *countingDiskUsager) DiskUsage(context.Context) (docker.DiskUsage, error) {
	c.calls++
	if c.err != nil {
		return docker.DiskUsage{}, c.err
	}
	return docker.DiskUsage{ImagesTotalBytes: int64(c.calls)}, nil
}

func TestCachedDiskUsager(t *testing.T) {
	inner := &countingDiskUsager{}
	cache := newCachedDiskUsager(inner, time.Minute)
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		u, err := cache.DiskUsage(ctx)
		if err != nil || u.ImagesTotalBytes != 1 {
			t.Fatalf("call %d: got %+v, %v", i, u, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls within ttl = %d, want 1", inner.calls)
	}

	now = now.Add(2 * time.Minute)
	if u, _ := cache.DiskUsage(ctx); u.ImagesTotalBytes != 2 || inner.calls != 2 {
		t.Fatalf("after ttl: got %+v, calls %d", u, inner.calls)
	}
}

func TestCachedDiskUsager_ErrorsNotCached(t *testing.T) {
	inner := &countingDiskUsager{err: errors.New("daemon down")}
	cache := newCachedDiskUsager(inner, time.Minute)
	for i := 0; i < 2; i++ {
		if _, err := cache.DiskUsage(context.Background()); err == nil {
			t.Fatal("want error")
		}
	}
	if inner.calls != 2 {
		t.Fatalf("inner calls = %d, want 2 (errors must not be cached)", inner.calls)
	}
}
