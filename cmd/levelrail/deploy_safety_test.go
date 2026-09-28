package main

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestPreviousReleaseHoldEnv(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", defaultPreviousReleaseHold},
		{"0", 0},
		{"90s", 90 * time.Second},
		{"garbage", defaultPreviousReleaseHold},
		{"-1m", defaultPreviousReleaseHold},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(previousReleaseHoldEnv, tt.value)
			if got := previousReleaseHold(logger); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
