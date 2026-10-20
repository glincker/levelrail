// Package ticker runs a function at a fixed interval until its context is
// cancelled, logging tick errors instead of stopping.
package ticker

import (
	"context"
	"log/slog"
	"time"
)

// Run calls tick on interval until ctx is done. A tick error is logged
// under label, never fatal: one failed tick must not stop the loop.
func Run(ctx context.Context, interval time.Duration, logger *slog.Logger, label string, tick func(context.Context) error) error {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := tick(ctx); err != nil {
				logger.Warn(label+": scheduler tick failed", slog.String("error", err.Error()))
			}
		}
	}
}
