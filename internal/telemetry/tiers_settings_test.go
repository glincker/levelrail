package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// An upgrade must keep today's history, so every active setting default equals the built-in config.
func TestTierSettingDefaultsMatchCurrentConfig(t *testing.T) {
	cfg := DefaultTierConfig()
	got := runtimesettings.New(nil, func(string) (string, bool) { return "", false }).MetricsTiers(context.Background())
	pairs := []struct {
		name      string
		got, want time.Duration
	}{
		{"raw retention", got.RawRetention, cfg.RawRetention},
		{"1m retention", got.Retention1m, cfg.MinuteRetention},
		{"1h retention", got.Retention1h, cfg.HourRetention},
		{"raw range", got.RawRange, cfg.RawMaxRange},
		{"1m range", got.Range1m, cfg.MinuteMaxRange},
		{"lag", got.Lag, cfg.Lag},
		{"rollup every", got.RollupEvery, time.Minute},
		{"prune every", got.PruneEvery, time.Hour},
	}
	for _, p := range pairs {
		if p.got != p.want {
			t.Errorf("%s default %s, built-in %s", p.name, p.got, p.want)
		}
	}
	if got.MaxDBBytes != cfg.MaxBytes {
		t.Errorf("size cap default %d, built-in %d", got.MaxDBBytes, cfg.MaxBytes)
	}
}

func TestTierConfigSourceAppliesWithoutRestart(t *testing.T) {
	t.Cleanup(func() { SetTierConfigSource(nil) })
	db := &DB{}
	live := 2 * time.Hour
	SetTierConfigSource(func(base TierConfig) TierConfig {
		base.RawMaxRange = live
		return base
	})
	if got := db.tierConfig().RawMaxRange; got != 2*time.Hour {
		t.Fatalf("got %s", got)
	}
	live = 4 * time.Hour
	if got := db.tierConfig().RawMaxRange; got != 4*time.Hour {
		t.Errorf("change not seen: %s", got)
	}
}
