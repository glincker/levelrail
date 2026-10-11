package telemetry

import "testing"

func TestParseSwapUsed(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
		ok   bool
	}{
		{"in use", "MemTotal: 100 kB\nSwapTotal:  2048 kB\nSwapFree:   1024 kB\n", 1024 * 1024, true},
		{"no swap configured", "SwapTotal: 0 kB\nSwapFree: 0 kB\n", 0, true},
		{"missing lines", "MemTotal: 100 kB\n", 0, false},
		{"garbage", "SwapTotal: x kB\nSwapFree: 1 kB\n", 0, false},
	}
	for _, tc := range tests {
		got, ok := parseSwapUsed([]byte(tc.in))
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: got %d %v, want %d %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
