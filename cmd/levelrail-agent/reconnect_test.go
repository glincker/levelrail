package main

import (
	"testing"
	"time"
)

func TestNextReconnect(t *testing.T) {
	zero := func(int64) int64 { return 0 }
	top := func(n int64) int64 { return n - 1 }
	tests := []struct {
		name     string
		cur      time.Duration
		lasted   time.Duration
		rnd      func(int64) int64
		wantWait time.Duration
		wantNext time.Duration
	}{
		{"first failure", reconnectBaseDelay, 0, zero, time.Second, 2 * time.Second},
		{"grows", 8 * time.Second, 0, zero, 8 * time.Second, 16 * time.Second},
		{"capped", reconnectMaxDelay, 0, zero, reconnectMaxDelay, reconnectMaxDelay},
		{"healthy session resets", reconnectMaxDelay, 2 * time.Minute, zero, time.Second, 2 * time.Second},
		{"jitter stays under 50 percent", 4 * time.Second, 0, top, 6 * time.Second, 8 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wait, next := nextReconnect(tc.cur, tc.lasted, tc.rnd)
			if wait != tc.wantWait || next != tc.wantNext {
				t.Fatalf("nextReconnect() = %v, %v, want %v, %v", wait, next, tc.wantWait, tc.wantNext)
			}
		})
	}
}
