package main

import (
	"log/slog"
	"testing"
	"time"
)

func TestSlowRequestThreshold(t *testing.T) {
	tests := []struct {
		name, raw string
		want      time.Duration
	}{
		{"unset falls back to 0 (api's own default)", "", 0},
		{"valid override", "250ms", 250 * time.Millisecond},
		{"invalid falls back to 0", "not-a-duration", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_SLOW_REQUEST_THRESHOLD", tt.raw)
			if got := slowRequestThreshold(slog.New(slog.DiscardHandler)); got != tt.want {
				t.Errorf("slowRequestThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCriticalRequestThreshold(t *testing.T) {
	tests := []struct {
		name, raw string
		want      time.Duration
	}{
		{"unset falls back to 0 (api's own default)", "", 0},
		{"valid override", "5s", 5 * time.Second},
		{"invalid falls back to 0", "not-a-duration", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_CRITICAL_REQUEST_THRESHOLD", tt.raw)
			if got := criticalRequestThreshold(slog.New(slog.DiscardHandler)); got != tt.want {
				t.Errorf("criticalRequestThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}
