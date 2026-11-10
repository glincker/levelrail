package catalog

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// FirstBootCeiling reads the live ceiling for a template's first_boot_seconds.
// A nil settings service resolves the env seed, then the default.
func FirstBootCeiling(ctx context.Context, s *runtimesettings.Service) int {
	return s.Int(ctx, runtimesettings.KeyTemplatesFirstBootMaxSeconds)
}

// firstBootCeiling is the submission-time bound, from the default or env seed.
func firstBootCeiling() int { return FirstBootCeiling(context.Background(), nil) }
