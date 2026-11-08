package models

import (
	"testing"
	"time"
)

func TestHFLimitsLive(t *testing.T) {
	SetHFLimitSource(func() (time.Duration, int64) { return 4 * time.Second, 2048 })
	t.Cleanup(func() { hfLimitsLive.Store(nil) })
	cfg := LoadHFConfig()
	if cfg.Timeout != 4*time.Second || cfg.MaxResponseBytes != 2048 {
		t.Errorf("cfg = %+v", cfg)
	}
	if got := NewHFClient(cfg, nil, nil).maxResponseBytes(); got != 2048 {
		t.Errorf("cap = %d", got)
	}
}
