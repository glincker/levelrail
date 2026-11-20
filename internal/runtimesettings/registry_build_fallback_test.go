package runtimesettings

import (
	"context"
	"testing"
	"time"
)

func TestBuildkitFallbackTimeouts_EnvSeedAndLiveRead(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{"APP_BUILDKIT_READY_TIMEOUT": "3m"}))
	ctx := context.Background()
	probe, ready, info, retry := svc.BuildkitFallbackTimeouts(ctx)
	if probe != 10*time.Second || ready != 3*time.Minute || info != 5*time.Second || retry != time.Second {
		t.Fatalf("seed/defaults = %s %s %s %s", probe, ready, info, retry)
	}
	if _, err := svc.Set(ctx, KeyBuildkitProbeTimeout, raw(`"1ms"`), "tester"); err == nil {
		t.Fatal("below floor accepted")
	}
	if _, err := svc.Set(ctx, KeyBuildkitProbeTimeout, raw(`"20s"`), "tester"); err != nil {
		t.Fatal(err)
	}
	if probe, _, _, _ = svc.BuildkitFallbackTimeouts(ctx); probe != 20*time.Second {
		t.Fatalf("saved should win live, got %s", probe)
	}
}
