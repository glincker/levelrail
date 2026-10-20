package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/profile"
)

func TestStartWhenWork(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Cleanup(func() { profile.SetActive(profile.Standard, profile.SourceDefault) })

	t.Run("standard starts at once", func(t *testing.T) {
		profile.SetActive(profile.Standard, profile.SourceDefault)
		var started atomic.Bool
		startWhenWork(t.Context(), logger, "x", func(context.Context) (bool, error) { return false, nil }, func() { started.Store(true) })
		if !started.Load() {
			t.Fatal("standard must not defer the loop")
		}
	})

	t.Run("minimal waits for work", func(t *testing.T) {
		profile.SetActive(profile.Minimal, profile.SourceDefault)
		t.Setenv(profile.EnvLazyStartInterval, "20ms")
		var work, started atomic.Bool
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		startWhenWork(ctx, logger, "x", func(context.Context) (bool, error) { return work.Load(), nil }, func() { started.Store(true) })
		time.Sleep(100 * time.Millisecond)
		if started.Load() {
			t.Fatal("started with no work")
		}
		work.Store(true)
		deadline := time.Now().Add(5 * time.Second)
		for !started.Load() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if !started.Load() {
			t.Fatal("never started once work appeared")
		}
	})
}
