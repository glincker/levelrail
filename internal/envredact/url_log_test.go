package envredact

import (
	"strings"
	"testing"
)

func TestURLForLog(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "https://github.com/acme/app", "https://github.com/acme/app"},
		{"password", "https://bob:s3cret@git.example.com/acme/app.git", "https://redacted@git.example.com/acme/app.git"},
		{"bare token user", "https://ghp_abc123@github.com/acme/app", "https://redacted@github.com/acme/app"},
		{"unparseable", "https://bad host:s3cret@x/%zz", "<unparseable url>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URLForLog(tt.in)
			if got != tt.want {
				t.Fatalf("URLForLog(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if strings.Contains(got, "s3cret") || strings.Contains(got, "ghp_abc123") {
				t.Fatalf("credential leaked: %q", got)
			}
		})
	}
}
