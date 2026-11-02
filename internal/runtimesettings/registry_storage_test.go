package runtimesettings

import (
	"context"
	"testing"
	"time"
)

func TestStorageSettings_SavedValueWinsLive(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{"APP_LOGS_RETENTION": "100h", "APP_DISK_LOW_FREE_MB": "2048"}))
	ctx := context.Background()
	if got := svc.Duration(ctx, KeyLogsRetention); got != 100*time.Hour {
		t.Fatalf("env seed = %s", got)
	}
	if _, err := svc.Set(ctx, KeyLogsRetention, raw(`"72h"`), "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Set(ctx, KeyDiskLowFreeMB, raw(`512`), "tester"); err != nil {
		t.Fatal(err)
	}
	if got := svc.Duration(ctx, KeyLogsRetention); got != 72*time.Hour {
		t.Fatalf("saved = %s", got)
	}
	if got := svc.Int(ctx, KeyDiskLowFreeMB); got != 512 {
		t.Fatalf("low = %d", got)
	}
}

func TestStorageSettings_RejectUnsafeValues(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	ctx := context.Background()
	bad := map[string]string{
		KeyMetricsRetention:   `"0s"`,
		KeyLogsRetention:      `"30m"`,
		KeyMonitorRawRetained: `"99999h"`,
		KeyLogBudgetMB:        `5`,
		KeyDiskLowFreeMB:      `0`,
		KeyDiskCriticalFreeMB: `-1`,
		KeyDiskReserveMB:      `99999`,
	}
	for key, val := range bad {
		if _, err := svc.Set(ctx, key, raw(val), "tester"); err == nil {
			t.Errorf("%s accepted %s", key, val)
		}
	}
	for key, val := range map[string]string{KeyLogBudgetMB: `0`, KeyDiskReserveMB: `0`, KeyMetricsRetention: `"1h"`} {
		if _, err := svc.Set(ctx, key, raw(val), "tester"); err != nil {
			t.Errorf("%s rejected %s: %v", key, val, err)
		}
	}
}
