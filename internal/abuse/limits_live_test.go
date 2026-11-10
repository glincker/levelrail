package abuse

import (
	"context"
	"testing"
)

func TestCurrentLimitsUsesLiveSource(t *testing.T) {
	t.Cleanup(func() { SetLiveLimits(nil) })
	want := DefaultLimits()
	want.MaxRPS = 7
	SetLiveLimits(func(context.Context) Limits { return want })
	if got := CurrentLimits(context.Background()).MaxRPS; got != 7 {
		t.Fatalf("live source ignored: %v", got)
	}
	SetLiveLimits(nil)
	if got := CurrentLimits(context.Background()).MaxRPS; got != DefaultLimits().MaxRPS {
		t.Fatalf("env fallback broken: %v", got)
	}
}
