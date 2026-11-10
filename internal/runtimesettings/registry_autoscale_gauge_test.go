package runtimesettings

import (
	"testing"
	"time"
)

func TestAutoscaleGaugeDefaultsAndEnvSeed(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	if got := svc.Duration(t.Context(), KeyAutoscaleGaugeTimeout); got != 3*time.Second {
		t.Errorf("timeout default = %v", got)
	}
	if got := svc.Limit(t.Context(), KeyAutoscaleGaugeMaxBytes); got != 1<<20 {
		t.Errorf("max bytes default = %d", got)
	}
	svc = New(newFakeStore(), envOf(map[string]string{"APP_AUTOSCALE_GAUGE_TIMEOUT": "5s", "APP_AUTOSCALE_GAUGE_MAX_BYTES": "0"}))
	if got := svc.Duration(t.Context(), KeyAutoscaleGaugeTimeout); got != 5*time.Second {
		t.Errorf("env seed = %v", got)
	}
	if got := svc.Limit(t.Context(), KeyAutoscaleGaugeMaxBytes); got != 1<<20 {
		t.Errorf("zero must not disable the cap, got %d", got)
	}
}
