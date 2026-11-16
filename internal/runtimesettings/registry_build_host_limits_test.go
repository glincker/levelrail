package runtimesettings

import (
	"context"
	"testing"
)

func TestBuildHostLimitSettings(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{"APP_BUILD_CPU_PERCENT": "50"}))
	ctx := context.Background()
	for key, def := range map[string]int{KeyBuildMemoryPercent: 75, KeyBuildkitdPidsLimit: 4096} {
		if got := svc.Int(ctx, key); got != def {
			t.Fatalf("%s default = %d, want %d", key, got, def)
		}
	}
	if got := svc.Int(ctx, KeyBuildCPUPercent); got != 50 {
		t.Fatalf("env seed = %d", got)
	}
	bad := map[string][]string{
		KeyBuildMemoryPercent: {`1`, `101`, `-1`},
		KeyBuildCPUPercent:    {`4`, `200`},
		KeyBuildkitdPidsLimit: {`10`, `99999999`},
	}
	for key, vals := range bad {
		for _, v := range vals {
			if _, err := svc.Set(ctx, key, raw(v), "tester"); err == nil {
				t.Fatalf("%s accepted %s", key, v)
			}
		}
		if _, err := svc.Set(ctx, key, raw(`0`), "tester"); err != nil {
			t.Fatalf("%s rejected 0: %v", key, err)
		}
	}
}
