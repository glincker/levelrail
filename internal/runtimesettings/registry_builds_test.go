package runtimesettings

import (
	"context"
	"testing"
)

func TestBuildsMemoryLimit_RangeAndLiveRead(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{"APP_BUILD_MEMORY_MB": "1024"}))
	ctx := context.Background()
	if got := svc.Int(ctx, KeyBuildsMemoryLimitMB); got != 1024 {
		t.Fatalf("env seed = %d", got)
	}
	for _, bad := range []string{`100`, `2000000`, `-5`} {
		if _, err := svc.Set(ctx, KeyBuildsMemoryLimitMB, raw(bad), "tester"); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
	if _, err := svc.Set(ctx, KeyBuildsMemoryLimitMB, raw(`2048`), "tester"); err != nil {
		t.Fatal(err)
	}
	if got := svc.Int(ctx, KeyBuildsMemoryLimitMB); got != 2048 {
		t.Fatalf("saved should win live, got %d", got)
	}
	if _, err := svc.Set(ctx, KeyBuildsMemoryLimitMB, raw(`0`), "tester"); err != nil {
		t.Fatalf("0 (auto) rejected: %v", err)
	}
}
