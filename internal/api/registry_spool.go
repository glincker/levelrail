package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/spool"
)

// spoolSweepEvery is the sweep cadence in the loop registry; the age and quota stay runtime settings.
const spoolSweepEvery = 10 * time.Minute

// RunSpoolSweeper deletes orphaned upload spool files left by a crash. Run it from the worker stage.
func RunSpoolSweeper(ctx context.Context, logger *slog.Logger) error {
	return footprint.Every(ctx, "upload spool sweep", spoolSweepEvery, logger, func(context.Context) error {
		if n := spool.Sweep(); n > 0 && logger != nil {
			logger.Info("upload spool: removed orphan files", slog.Int("removed", n))
		}
		return nil
	})
}
