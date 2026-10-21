package telemetry

import (
	"testing"
	"time"
)

func TestTrafficRetentionFromEnv(t *testing.T) {
	if got := TrafficRetentionFromEnv(time.Hour); got != time.Hour {
		t.Fatalf("default = %v", got)
	}
	t.Setenv(EnvTrafficRetention, "36h")
	if got := TrafficRetentionFromEnv(time.Hour); got != 36*time.Hour {
		t.Fatalf("override = %v", got)
	}
}
