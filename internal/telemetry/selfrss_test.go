package telemetry

import "testing"

func TestParseStatmRSS(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"1000 250 10 1 0 5 0", 250 * 4096, true},
		{"1000", 0, false},
		{"1000 x", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseStatmRSS(tt.in, 4096)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("%q: got %d %v", tt.in, got, ok)
		}
	}
}
