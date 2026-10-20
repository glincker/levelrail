package version

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		injected string
		module   string
		want     string
	}{
		{"ldflags wins over module info", "v0.2.0-beta.15", "v9.9.9", "v0.2.0-beta.15"},
		{"go install uses module version", "dev", "v0.2.0-beta.15", "v0.2.0-beta.15"},
		{"checkout build stays dev", "dev", "(devel)", "dev"},
		{"empty module info stays dev", "dev", "", "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.injected, tt.module); got != tt.want {
				t.Fatalf("resolve(%q, %q) = %q, want %q", tt.injected, tt.module, got, tt.want)
			}
		})
	}
}
