package runtimesettings

import (
	"testing"
	"time"
)

func TestMeshApplyClampsOutOfRangeEnvSeeds(t *testing.T) {
	cases := []struct {
		name                     string
		env                      map[string]string
		debounce, timeout, delay time.Duration
		retries                  int
	}{
		// A zero duration is not a valid env seed, so the default stands.
		{"zero", map[string]string{
			"APP_MESH_WAKE_DEBOUNCE": "0s", "APP_MESH_APPLY_TIMEOUT": "0s",
			"APP_MESH_APPLY_RETRY_DELAY": "0s", "APP_MESH_APPLY_RETRIES": "0",
		}, time.Second, 10 * time.Second, 500 * time.Millisecond, 0},
		{"tiny", map[string]string{
			"APP_MESH_WAKE_DEBOUNCE": "1ms", "APP_MESH_APPLY_TIMEOUT": "1ms",
			"APP_MESH_APPLY_RETRY_DELAY": "1ms", "APP_MESH_APPLY_RETRIES": "-3",
		}, meshWakeDebounceMin, meshApplyTimeoutMin, meshRetryDelayMin, 0},
		{"huge", map[string]string{
			"APP_MESH_WAKE_DEBOUNCE": "100h", "APP_MESH_APPLY_TIMEOUT": "100h",
			"APP_MESH_APPLY_RETRY_DELAY": "100h", "APP_MESH_APPLY_RETRIES": "1000",
		}, meshWakeDebounceMax, meshApplyTimeoutMax, meshRetryDelayMax, meshApplyRetriesMax},
		{"defaults", nil, time.Second, 10 * time.Second, 500 * time.Millisecond, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(newFakeStore(), envOf(tc.env))
			ctx := t.Context()
			if got := ClampMeshWakeDebounce(svc.Duration(ctx, KeyMeshWakeDebounce)); got != tc.debounce {
				t.Errorf("debounce = %v, want %v", got, tc.debounce)
			}
			if got := ClampMeshApplyTimeout(svc.Duration(ctx, KeyMeshApplyTimeout)); got != tc.timeout {
				t.Errorf("timeout = %v, want %v", got, tc.timeout)
			}
			if got := ClampMeshApplyRetryDelay(svc.Duration(ctx, KeyMeshApplyRetryDelay)); got != tc.delay {
				t.Errorf("retry delay = %v, want %v", got, tc.delay)
			}
			if got := ClampMeshApplyRetries(svc.Int(ctx, KeyMeshApplyRetries)); got != tc.retries {
				t.Errorf("retries = %d, want %d", got, tc.retries)
			}
		})
	}
}
