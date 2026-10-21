package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	bootDockerCallTimeout = 10 * time.Second
	eventRetryMin         = time.Second
	eventRetryMax         = 30 * time.Second
)

// resilientEvents re-subscribes to the Docker event stream with backoff, so
// the reconciler recovers when the daemon restarts or was down at boot.
func resilientEvents(ctx context.Context, subscribe func(context.Context) (<-chan docker.Event, <-chan error), logger *slog.Logger) <-chan docker.Event {
	out := make(chan docker.Event)
	go func() {
		defer close(out)
		wait := eventRetryMin
		for ctx.Err() == nil {
			events, errs := subscribe(ctx)
			start := time.Now()
			for ev := range events {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
			if err, ok := <-errs; ok && err != nil {
				logger.Error("docker event stream error", slog.String("error", err.Error()))
			}
			if ctx.Err() != nil {
				return
			}
			if time.Since(start) > eventRetryMax {
				wait = eventRetryMin
			}
			logger.Warn("docker event stream ended, resubscribing", slog.Duration("in", wait))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = min(wait*2, eventRetryMax)
		}
	}()
	return out
}
