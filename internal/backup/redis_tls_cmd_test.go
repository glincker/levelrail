package backup

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRedisTLSProbe(t *testing.T) {
	tests := []struct {
		name  string
		setup string
		want  string
	}{
		{"no certs mounted uses plaintext", `[ -f /nonexistent/tls.crt ]`, ""},
		{"certs mounted switches to tls port", `true`, "--tls --insecure -p 6380"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := strings.Replace(redisTLSProbe, "[ -f /certs/tls.crt ]", tt.setup, 1) + ` printf %s "$RTLS"`
			out, err := exec.Command("sh", "-c", script).Output() //nolint:gosec // script is built from package constants in a test
			if err != nil {
				t.Fatalf("sh: %v", err)
			}
			if string(out) != tt.want {
				t.Fatalf("RTLS = %q, want %q", out, tt.want)
			}
		})
	}
}
