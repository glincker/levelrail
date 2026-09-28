package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b   string
		want   int
		wantOK bool
	}{
		{"v1.2.3", "1.2.3", 0, true},
		{"v1.2.3", "v1.10.0", -1, true},
		{"2.0", "1.9.9", 1, true},
		{"0.4.0-beta.2", "0.4.0", -1, true},
		{"0.4.0-beta.10", "0.4.0-beta.2", 1, true},
		{"0.4.0-alpha", "0.4.0-beta", -1, true},
		{"0.4.0-1", "0.4.0-alpha", -1, true},
		{"0.4.0-beta", "0.4.0-beta.1", -1, true},
		{"1.0.0+build.5", "1.0.0", 0, true},
		{"dev", "1.0.0", 0, false},
		{"1.0.0", "", 0, false},
		{"1", "1.0.0", 0, false},
		{"1.x.0", "1.0.0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got, ok := Compare(tt.a, tt.b)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Compare(%q, %q) = %d, %v, want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
